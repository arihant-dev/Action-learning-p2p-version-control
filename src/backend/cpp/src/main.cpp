#include "filesystem_watcher.h"
#include "ipc_client.h"
#include "sha256.h"
#include "file_transfer.h"

#include <atomic>
#include <csignal>
#include <filesystem>
#include <iostream>
#include <nlohmann/json.hpp>
#include <string>
#include <thread>
#include <chrono>

namespace fs = std::filesystem;

std::atomic<bool> g_shutdown(false);

void signal_handler(int signal) {
    std::cout << "\n[C++ Daemon] Received signal: " << signal << ", shutting down...\n";
    g_shutdown = true;
}

void print_usage(const char *program_name) {
    std::cout << "Usage: " << program_name << " <repo_id> <watch_path> [ipc_socket_path]\n";
    std::cout << "Example: " << program_name << " project-alpha /Users/arihant/sync /tmp/p2p_sync.sock\n";
}

void handle_ipc_message(const nlohmann::json &msg, const std::string &watch_path) {
    try {
        std::string msg_type = msg.value("type", "");
        std::cout << "[C++ Daemon] Received IPC message: " << msg_type << "\n";
        
        if (msg_type == "prepare_file_transfer") {
            auto payload = msg.at("payload");
            std::string transfer_id = payload.value("transfer_id", "");
            std::string path = payload.value("path", "");
            std::string peer_id = payload.value("peer_id", "");
            int transfer_port = payload.value("transfer_port", 0);
            std::string expected_hash = payload.value("expected_hash", "");
            long long expected_size = payload.value("expected_size", 0LL);
            std::string direction = payload.value("direction", "download");

            std::cout << "[C++ Daemon] Handled prepare_file_transfer: ID=" << transfer_id 
                      << ", path=" << path << ", port=" << transfer_port 
                      << ", dir=" << direction << "\n";

            // Spawn background thread to perform transfer
            std::thread([=]() {
                transfer::handle_file_transfer(watch_path, path, transfer_port, direction, expected_size, expected_hash);
            }).detach();
        } 
        else if (msg_type == "sync_from_peer") {
            auto payload = msg.at("payload");
            std::string path = payload.value("path", "");
            bool is_delete = payload.value("is_delete", false);

            std::cout << "[C++ Daemon] Handled sync_from_peer: path=" << path 
                      << ", is_delete=" << is_delete << "\n";

            if (is_delete) {
                fs::path target_path = fs::path(watch_path) / path;
                try {
                    if (fs::exists(target_path)) {
                        fs::remove(target_path);
                        std::cout << "[C++ Daemon] Deleted file locally: " << target_path << "\n";
                    }
                } catch (const std::exception &e) {
                    std::cerr << "[C++ Daemon] Error deleting file: " << e.what() << "\n";
                }
            }
        }
    } catch (const std::exception &e) {
        std::cerr << "[C++ Daemon] Error handling IPC message: " << e.what() << "\n";
    }
}

int main(int argc, char *argv[]) {
    if (argc < 2) {
        print_usage(argv[0]);
        return 1;
    }

    std::string repo_id = "project-alpha";
    std::string watch_path = "";
    std::string ipc_socket = "/tmp/p2p_sync.sock";

    if (argc == 2) {
        watch_path = argv[1];
    } else {
        repo_id = argv[1];
        watch_path = argv[2];
        if (argc >= 4) {
            ipc_socket = argv[3];
        }
    }

    if (const char* env_ipc = std::getenv("IPC_SOCKET")) {
        ipc_socket = env_ipc;
    }

    // Validate path exists
    if (!fs::exists(watch_path)) {
        std::cerr << "[C++ Daemon] Error: Watch path does not exist: " << watch_path << "\n";
        return 1;
    }

    if (!fs::is_directory(watch_path)) {
        std::cerr << "[C++ Daemon] Error: Watch path is not a directory: " << watch_path << "\n";
        return 1;
    }

    std::cout << "==========================================\n";
    std::cout << " P2P File Sync - C++ Daemon (Cross-Platform)\n";
    std::cout << " Repository ID   : " << repo_id << "\n";
    std::cout << " Watch Directory : " << watch_path << "\n";
    std::cout << " IPC Socket Path : " << ipc_socket << "\n";
    std::cout << "==========================================\n";

    // Setup signal handlers
    std::signal(SIGINT, signal_handler);
    std::signal(SIGTERM, signal_handler);

    // Initialize IPC Client
    ipc::IpcClient ipc_client;
    
    // Connect and read messages in a background thread so the daemon can start watching
    // immediately and handle incoming messages when connected
    std::thread ipc_thread([&]() {
        std::cout << "[C++ Daemon] Background IPC worker started...\n";
        while (!g_shutdown) {
            if (!ipc_client.is_connected()) {
                if (!ipc_client.connect(ipc_socket)) {
                    std::this_thread::sleep_for(std::chrono::seconds(2));
                    continue;
                }
            }

            // Connection succeeded, enter read loop
            while (ipc_client.is_connected() && !g_shutdown) {
                nlohmann::json message;
                if (ipc_client.read_message(message)) {
                    handle_ipc_message(message, watch_path);
                } else {
                    // read_message disconnects on EOF/error
                    std::this_thread::sleep_for(std::chrono::milliseconds(100));
                }
            }
        }
    });

    // Create file watcher
    FileSystemWatcher watcher(watch_path, g_shutdown);

    // Callback for file changes
    auto on_file_change = [&](const std::string &rel_path, const std::string &action) {
        std::cout << "[C++ Daemon] Event detected: " << action << " -> " << rel_path << "\n";

        // Print [JSON] line for test compatibility
        nlohmann::json test_json;
        test_json["filename"] = rel_path;
        test_json["action"] = (action == "add") ? "created" : ((action == "delete") ? "deleted" : "modified");
        std::cout << "[JSON] " << test_json.dump() << "\n" << std::flush;

        fs::path abs_path = fs::path(watch_path) / rel_path;
        
        long long size = 0;
        std::string hash = "";
        long long modified_time = std::chrono::duration_cast<std::chrono::seconds>(
            std::chrono::system_clock::now().time_since_epoch()
        ).count();

        if (action != "delete" && fs::exists(abs_path)) {
            try {
                size = fs::file_size(abs_path);
                hash = crypto::sha256_file(abs_path.string());
                
                auto write_time = fs::last_write_time(abs_path);
                auto sctp = std::chrono::time_point_cast<std::chrono::system_clock::duration>(
                    write_time - fs::file_time_type::clock::now() + std::chrono::system_clock::now()
                );
                modified_time = std::chrono::duration_cast<std::chrono::seconds>(sctp.time_since_epoch()).count();
            } catch (const std::exception &e) {
                std::cerr << "[C++ Daemon] Error processing file metadata: " << e.what() << "\n";
                return; // Don't notify if we failed to read file metadata
            }
        }

        // Package into standard Go/C++ IPC message contract
        nlohmann::json message;
        message["version"] = "1.0";
        message["type"] = "file_changed";
        message["id"] = "msg_cpp_" + std::to_string(std::chrono::system_clock::now().time_since_epoch().count());
        message["timestamp"] = std::chrono::duration_cast<std::chrono::milliseconds>(
            std::chrono::system_clock::now().time_since_epoch()
        ).count();
        message["source"] = "cpp";

        nlohmann::json payload;
        payload["repo_id"] = repo_id;
        payload["action"] = action;
        payload["path"] = rel_path;
        payload["hash"] = hash;
        payload["size"] = size;
        payload["modified_time"] = modified_time;

        message["payload"] = payload;

        if (ipc_client.is_connected()) {
            std::cout << "[C++ Daemon] Sending IPC change: " << payload.dump() << "\n";
            ipc_client.send_message(message);
        } else {
            std::cout << "[C++ Daemon] IPC disconnected, change not sent: " << payload.dump() << "\n";
        }
    };

    // Start watching
    if (!watcher.start(on_file_change)) {
        std::cerr << "[C++ Daemon] Error: Failed to start file watcher\n";
        g_shutdown = true;
    }

    // Keep running
    while (!g_shutdown) {
        std::this_thread::sleep_for(std::chrono::milliseconds(200));
    }

    watcher.stop();
    
    // Stop IPC connection thread
    if (ipc_thread.joinable()) {
        ipc_thread.join();
    }
    
    ipc_client.disconnect();
    std::cout << "[C++ Daemon] Exited cleanly.\n";

    return 0;
}
