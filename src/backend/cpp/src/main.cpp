#include "filesystem_watcher.h"
#include "ipc_client.h"
#include "sha256.h"
#include "file_transfer.h"

#include <atomic>
#include <csignal>
#include <cstring>
#include <cstdlib>
#include <filesystem>
#include <iostream>
#include <nlohmann/json.hpp>
#include <string>
#include <thread>
#include <chrono>
#include <fstream>

#ifdef _WIN32
#include <io.h>
#define STDERR_FILENO 2
#else
#include <unistd.h>
#endif

namespace fs = std::filesystem;

std::atomic<bool> g_shutdown(false);

void signal_handler(int) {
    const char msg[] = "\n[C++ Daemon] Received signal, shutting down...\n";
    (void)::write(STDERR_FILENO, msg, sizeof(msg) - 1);
    g_shutdown = true;
}

void print_usage(const char* program_name) {
    std::cout << "Usage: " << program_name
              << " <repo_id> <watch_path> [ipc_socket_path] [--poll-interval <ms>]\n";
    std::cout << "Example: " << program_name
              << " project-alpha /path/to/watch /tmp/p2p_sync.sock --poll-interval 500\n";
}

std::string normalize_path(const std::string& path) {
    std::string result = path;
    for (auto& ch : result) {
        if (ch == '\\') ch = '/';
    }
    return result;
}

void handle_ipc_message(const nlohmann::json& msg, const std::string& my_repo_id,
                        const std::string& watch_path) {
    try {
        std::string msg_type = msg.value("type", "");
        std::cout << "[C++ Daemon] Received IPC message: " << msg_type << "\n";

        if (msg_type == "prepare_file_transfer") {
            auto payload = msg.at("payload");
            std::string msg_repo_id = payload.value("repo_id", "");
            if (!msg_repo_id.empty() && msg_repo_id != my_repo_id) return;

            std::string transfer_id = payload.value("transfer_id", "");
            std::string path = normalize_path(payload.value("path", ""));
            std::string peer_id = payload.value("peer_id", "");
            int transfer_port = payload.value("transfer_port", 0);
            std::string expected_hash = payload.value("expected_hash", "");
            long long expected_size = payload.value("expected_size", 0LL);
            std::string direction = payload.value("direction", "download");
            uint32_t mode = payload.value("mode", 0);

            std::cout << "[C++ Daemon] prepare_file_transfer: ID=" << transfer_id
                      << ", path=" << path << ", port=" << transfer_port
                      << ", dir=" << direction << "\n";

            std::thread([=]() {
                transfer::handle_file_transfer(
                    watch_path, path, transfer_port, direction,
                    expected_size, expected_hash, mode);
            }).detach();
        } else if (msg_type == "sync_from_peer") {
            auto payload = msg.at("payload");
            std::string msg_repo_id = payload.value("repo_id", "");
            if (!msg_repo_id.empty() && msg_repo_id != my_repo_id) return;

            std::string path = normalize_path(payload.value("path", ""));
            bool is_delete = payload.value("is_delete", false);

            std::cout << "[C++ Daemon] sync_from_peer: path=" << path
                      << ", is_delete=" << is_delete << "\n";

            if (is_delete) {
                fs::path target_path = fs::path(watch_path) / path;
                try {
                    if (fs::exists(target_path)) {
                        fs::remove(target_path);
                        std::cout << "[C++ Daemon] Deleted: " << target_path << "\n";
                    }
                } catch (const std::exception& e) {
                    std::cerr << "[C++ Daemon] Delete error: " << e.what() << "\n";
                }
            }
        }
    } catch (const std::exception& e) {
        std::cerr << "[C++ Daemon] IPC handler error: " << e.what() << "\n";
    }
}

int main(int argc, char* argv[]) {
    if (argc < 2) {
        print_usage(argv[0]);
        return 1;
    }

    std::string repo_id = "project-alpha";
    std::string watch_path;
    std::string ipc_socket = "/tmp/p2p_sync.sock";
    int poll_interval_ms = 1000;

    int i = 1;
    if (argc >= 2) {
        repo_id = argv[i++];
    }
    if (argc >= 3) {
        watch_path = argv[i++];
    }

    while (i < argc) {
        if (std::strcmp(argv[i], "--poll-interval") == 0 && i + 1 < argc) {
            poll_interval_ms = std::atoi(argv[++i]);
            if (poll_interval_ms <= 0) poll_interval_ms = 1000;
            ++i;
        } else {
            ipc_socket = argv[i++];
        }
    }

    if (const char* env_ipc = std::getenv("IPC_SOCKET")) {
        ipc_socket = env_ipc;
    }

    if (!fs::exists(watch_path)) {
        std::cerr << "[C++ Daemon] Error: Watch path does not exist: " << watch_path << "\n";
        return 1;
    }
    if (!fs::is_directory(watch_path)) {
        std::cerr << "[C++ Daemon] Error: Not a directory: " << watch_path << "\n";
        return 1;
    }

    std::cout << "==========================================\n";
    std::cout << " P2P File Sync - C++ Daemon\n";
    std::cout << " Repository ID   : " << repo_id << "\n";
    std::cout << " Watch Directory : " << watch_path << "\n";
    std::cout << " IPC Socket Path : " << ipc_socket << "\n";
    std::cout << " Poll Interval   : " << poll_interval_ms << " ms\n";
    std::cout << "==========================================\n";

    std::signal(SIGINT, signal_handler);
    std::signal(SIGTERM, signal_handler);
#ifndef _WIN32
    std::signal(SIGPIPE, SIG_IGN);
#endif

    auto ipc_client = IpcClient::create();

    std::thread ipc_thread([&]() {
        std::cout << "[C++ Daemon] IPC worker started\n";
        while (!g_shutdown) {
            if (!ipc_client->isConnected()) {
                if (!ipc_client->connect(ipc_socket)) {
                    std::this_thread::sleep_for(std::chrono::seconds(2));
                    continue;
                }
            }
            while (ipc_client->isConnected() && !g_shutdown) {
                std::string raw;
                if (ipc_client->receive(raw)) {
                    try {
                        auto msg = nlohmann::json::parse(raw);
                        handle_ipc_message(msg, repo_id, watch_path);
                    } catch (...) {}
            } else {
                if (g_shutdown) break;
                std::this_thread::sleep_for(std::chrono::milliseconds(100));
            }
            }
        }
    });

    auto watcher = createWatcher(
        watch_path,
        [&](const WatchEvent& event) {
            std::string action;
            switch (event.type) {
                case WatchEventType::Created: action = "created"; break;
                case WatchEventType::Modified: action = "modified"; break;
                case WatchEventType::Deleted: action = "deleted"; break;
                case WatchEventType::Moved: action = "moved"; break;
            }

            std::cout << "[C++ Daemon] Event: " << action << " -> " << event.path << "\n";

            std::string normPath = normalize_path(event.path);

            // Skip .tmp files (transfer internals) to prevent metadata races.
            if (normPath.size() >= 4 && normPath.substr(normPath.size() - 4) == ".tmp") {
                return;
            }

            nlohmann::json test_json;
            test_json["filename"] = normPath;
            test_json["action"] = action;
            std::cout << "[JSON] " << test_json.dump() << "\n" << std::flush;

            fs::path abs_path = fs::path(watch_path) / normPath;

            long long size = 0;
            std::string hash;
            long long modified_time = std::chrono::duration_cast<std::chrono::seconds>(
                std::chrono::system_clock::now().time_since_epoch()
            ).count();
            uint32_t mode = 0;

            if (event.type != WatchEventType::Deleted && fs::exists(abs_path)) {
                try {
                    // Read file content once to compute both size and hash
                    // atomically, avoiding TOCTOU races where the file is
                    // modified between the size and hash calls.
                    {
                        std::ifstream file(abs_path.string(), std::ios::binary | std::ios::ate);
                        if (file) {
                            size = file.tellg();
                            file.seekg(0, std::ios::beg);
                            std::string content(static_cast<std::size_t>(size), '\0');
                            if (file.read(content.data(), static_cast<std::streamsize>(size))) {
                                hash = crypto::sha256(content);
                            }
                        }
                    }
                    auto write_time = fs::last_write_time(abs_path);
                    #ifdef _WIN32
                    auto sctp = std::chrono::clock_cast<std::chrono::system_clock>(write_time);
                    modified_time = std::chrono::duration_cast<std::chrono::seconds>(
                        sctp.time_since_epoch()).count();
                    #else
                    auto sctp = std::chrono::file_clock::to_sys(write_time);
                    modified_time = std::chrono::duration_cast<std::chrono::seconds>(
                        sctp.time_since_epoch()).count();
                    #endif
                    auto perms = fs::status(abs_path).permissions();
                    mode = static_cast<uint32_t>(perms);
                } catch (const std::exception& e) {
                    std::cerr << "[C++ Daemon] Metadata error: " << e.what() << "\n";
                    return;
                }
            }

            nlohmann::json message;
            message["version"] = "1.0";
            message["type"] = "file_changed";
            message["id"] = "msg_cpp_" +
                std::to_string(std::chrono::system_clock::now().time_since_epoch().count());
            message["timestamp"] = std::chrono::duration_cast<std::chrono::milliseconds>(
                std::chrono::system_clock::now().time_since_epoch()
            ).count();
            message["source"] = "cpp";

            nlohmann::json payload;
            payload["repo_id"] = repo_id;
            payload["action"] = action;
            payload["path"] = normPath;
            payload["hash"] = hash;
            payload["size"] = size;
            payload["modified_time"] = modified_time;
            payload["mode"] = mode;
            message["payload"] = payload;

            if (ipc_client->isConnected()) {
                std::cout << "[C++ Daemon] Sending IPC change\n";
                if (!ipc_client->send(message.dump())) {
                    std::cout << "[C++ Daemon] IPC send failed\n";
                }
            } else {
                std::cout << "[C++ Daemon] IPC disconnected, change not sent\n";
            }
        },
        std::chrono::milliseconds(poll_interval_ms));

    if (!watcher || !watcher->start()) {
        std::cerr << "[C++ Daemon] Failed to start file watcher\n";
        g_shutdown = true;
    }

    while (!g_shutdown) {
        std::this_thread::sleep_for(std::chrono::milliseconds(200));
    }

    if (watcher) watcher->stop();
    if (ipc_thread.joinable()) ipc_thread.join();
    ipc_client->disconnect();
    std::cout << "[C++ Daemon] Exited cleanly.\n";

    return 0;
}
