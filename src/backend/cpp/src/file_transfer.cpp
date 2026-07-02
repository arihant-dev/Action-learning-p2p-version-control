#include "file_transfer.h"
#include "sha256.h"

#include <iostream>
#include <fstream>
#include <filesystem>
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <unistd.h>
#include <cstring>

namespace fs = std::filesystem;

namespace transfer {

void handle_file_transfer(
    const std::string &watch_path,
    const std::string &rel_path,
    int port,
    const std::string &direction,
    long long expected_size,
    const std::string &expected_hash
) {
    std::cout << "[C++ Daemon] Starting file transfer. Path: " << rel_path 
              << ", Port: " << port << ", Direction: " << direction << "\n";

    // 1. Create socket
    int sock_fd = ::socket(AF_INET, SOCK_STREAM, 0);
    if (sock_fd < 0) {
        std::cerr << "[C++ Daemon] Error: Failed to create transfer socket\n";
        return;
    }

    // 2. Setup address (127.0.0.1:port)
    struct sockaddr_in addr;
    std::memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_port = htons(port);
    if (::inet_pton(AF_INET, "127.0.0.1", &addr.sin_addr) <= 0) {
        std::cerr << "[C++ Daemon] Error: Invalid transfer address\n";
        ::close(sock_fd);
        return;
    }

    // 3. Connect to Go coordinator's local port
    if (::connect(sock_fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        std::cerr << "[C++ Daemon] Error: Failed to connect to transfer port " << port << "\n";
        ::close(sock_fd);
        return;
    }

    std::cout << "[C++ Daemon] Connected to transfer socket at port " << port << "\n";

    fs::path dest_path = fs::path(watch_path) / rel_path;

    if (direction == "download") {
        fs::path tmp_path = dest_path.string() + ".tmp";
        try {
            // Ensure parent directories exist
            fs::create_directories(dest_path.parent_path());

            std::ofstream outfile(tmp_path, std::ios::binary);
            if (!outfile) {
                std::cerr << "[C++ Daemon] Error: Failed to open temp output file: " << tmp_path << "\n";
                ::close(sock_fd);
                return;
            }

            char buffer[4096];
            long long total_received = 0;
            while (total_received < expected_size || expected_size <= 0) {
                long long to_read = (expected_size > 0) ? std::min(4096LL, expected_size - total_received) : 4096LL;
                if (to_read == 0) break;

                ssize_t n = ::read(sock_fd, buffer, to_read);
                if (n < 0) {
                    std::cerr << "[C++ Daemon] Error: Socket read error during download\n";
                    break;
                }
                if (n == 0) {
                    // EOF reached
                    break;
                }

                outfile.write(buffer, n);
                total_received += n;
            }

            outfile.close();

            if (total_received != expected_size && expected_size > 0) {
                std::cerr << "[C++ Daemon] Error: Downloaded size mismatch (got " << total_received 
                          << ", expected " << expected_size << "). Aborting.\n";
                fs::remove(tmp_path);
                ::close(sock_fd);
                return;
            }

            // Verify hash if expected_hash is provided
            if (!expected_hash.empty()) {
                std::string actual_hash = crypto::sha256_file(tmp_path.string());
                if (actual_hash != expected_hash) {
                    std::cerr << "[C++ Daemon] Error: Downloaded hash mismatch for " << rel_path 
                              << " (got " << actual_hash << ", expected " << expected_hash << "). Aborting.\n";
                    fs::remove(tmp_path);
                    ::close(sock_fd);
                    return;
                }
            }

            // Atomic rename to replace the target file
            fs::rename(tmp_path, dest_path);
            std::cout << "[C++ Daemon] Download completed atomically. Target: " << dest_path << "\n";
        } catch (const std::exception &e) {
            std::cerr << "[C++ Daemon] Exception during download: " << e.what() << "\n";
            if (fs::exists(tmp_path)) {
                fs::remove(tmp_path);
            }
        }
    } 
    else if (direction == "upload") {
        try {
            if (!fs::exists(dest_path)) {
                std::cerr << "[C++ Daemon] Error: Upload file does not exist: " << dest_path << "\n";
                ::close(sock_fd);
                return;
            }

            std::ifstream infile(dest_path, std::ios::binary);
            if (!infile) {
                std::cerr << "[C++ Daemon] Error: Failed to open input file: " << dest_path << "\n";
                ::close(sock_fd);
                return;
            }

            char buffer[4096];
            long long total_sent = 0;
            while (infile && (total_sent < expected_size || expected_size <= 0)) {
                infile.read(buffer, sizeof(buffer));
                ssize_t bytes_to_send = infile.gcount();
                if (bytes_to_send <= 0) break;

                ssize_t written = 0;
                while (written < bytes_to_send) {
                    ssize_t n = ::write(sock_fd, buffer + written, bytes_to_send - written);
                    if (n <= 0) {
                        std::cerr << "[C++ Daemon] Error: Socket write error during upload\n";
                        break;
                    }
                    written += n;
                }
                total_sent += written;
            }

            infile.close();
            std::cout << "[C++ Daemon] Upload completed. Sent " << total_sent << " bytes for " << rel_path << "\n";
        } catch (const std::exception &e) {
            std::cerr << "[C++ Daemon] Exception during upload: " << e.what() << "\n";
        }
    }

    ::close(sock_fd);
}

} // namespace transfer
