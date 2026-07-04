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
#include <cerrno>

namespace fs = std::filesystem;

namespace transfer {

void handle_file_transfer(
    const std::string &watch_path,
    const std::string &rel_path,
    int port,
    const std::string &direction,
    long long expected_size,
    const std::string &expected_hash,
    uint32_t mode
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

    // Set socket timeouts (30s) so read()/write() can't hang forever
    struct timeval sock_timeout;
    sock_timeout.tv_sec = 30;
    sock_timeout.tv_usec = 0;
    ::setsockopt(sock_fd, SOL_SOCKET, SO_RCVTIMEO, &sock_timeout, sizeof(sock_timeout));
    ::setsockopt(sock_fd, SOL_SOCKET, SO_SNDTIMEO, &sock_timeout, sizeof(sock_timeout));

    std::cout << "[C++ Daemon] Connected to transfer socket at port " << port << "\n";

    // Validate expected_size to prevent infinite loops
    if (expected_size <= 0) {
        std::cerr << "[C++ Daemon] Error: Invalid expected_size " << expected_size << " for " << direction << "\n";
        ::close(sock_fd);
        return;
    }

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
            while (total_received < expected_size) {
                long long to_read = std::min(4096LL, expected_size - total_received);
                if (to_read <= 0) break;

                ssize_t n = ::read(sock_fd, buffer, to_read);
                if (n < 0) {
                    if (errno == EAGAIN || errno == EWOULDBLOCK) {
                        std::cerr << "[C++ Daemon] Error: Socket read timeout during download\n";
                    } else {
                        std::cerr << "[C++ Daemon] Error: Socket read error during download: " << errno << "\n";
                    }
                    break;
                }
                if (n == 0) {
                    // EOF reached before expected_size
                    break;
                }

                outfile.write(buffer, n);
                if (outfile.fail()) {
                    std::cerr << "[C++ Daemon] Error: Write failed to temp file: " << tmp_path << "\n";
                    break;
                }
                total_received += n;
            }

            outfile.close();

            if (total_received != expected_size) {
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

            // Apply file permissions if mode is provided
            if (mode > 0) {
                try {
                    fs::permissions(dest_path, static_cast<fs::perms>(mode));
                    std::cout << "[C++ Daemon] Applied file permissions " << std::oct << mode << " to " << dest_path << "\n";
                } catch (const std::exception &e) {
                    std::cerr << "[C++ Daemon] Warning: Failed to set permissions: " << e.what() << "\n";
                }
            }

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
            while (total_sent < expected_size) {
                long long to_read = std::min(4096LL, expected_size - total_sent);
                infile.read(buffer, to_read);
                ssize_t bytes_to_send = static_cast<ssize_t>(infile.gcount());
                if (bytes_to_send <= 0) break;

                ssize_t written = 0;
                while (written < bytes_to_send) {
                    ssize_t n = ::write(sock_fd, buffer + written, bytes_to_send - written);
                    if (n < 0) {
                        if (errno == EAGAIN || errno == EWOULDBLOCK) {
                            std::cerr << "[C++ Daemon] Error: Socket write timeout during upload\n";
                        } else {
                            std::cerr << "[C++ Daemon] Error: Socket write error during upload: " << errno << "\n";
                        }
                        break;
                    }
                    written += n;
                }
                if (written < bytes_to_send) break; // break outer loop on write error
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
