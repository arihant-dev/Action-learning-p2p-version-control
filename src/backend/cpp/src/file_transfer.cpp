#include "file_transfer.h"
#include "sha256.h"

#include <iostream>
#include <fstream>
#include <filesystem>
#include <cstring>
#include <cerrno>
#include <vector>

#ifdef _WIN32
#include <winsock2.h>
#include <ws2tcpip.h>
#pragma comment(lib, "ws2_32.lib")
#else
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <unistd.h>
#endif

namespace fs = std::filesystem;

namespace transfer {

static void closeSocket(int fd) {
#ifdef _WIN32
    closesocket(fd);
#else
    ::close(fd);
#endif
}

void handle_file_transfer(
    const std::string& watch_path,
    const std::string& rel_path,
    int port,
    const std::string& direction,
    long long expected_size,
    const std::string& expected_hash,
    uint32_t mode
) {
    std::cout << "[C++ Daemon] File transfer: " << rel_path
              << " port=" << port << " dir=" << direction << "\n";

#ifdef _WIN32
    WSADATA wsaData;
    WSAStartup(MAKEWORD(2, 2), &wsaData);
#endif

    int sock_fd = ::socket(AF_INET, SOCK_STREAM, 0);
    if (sock_fd < 0) {
        std::cerr << "[C++ Daemon] Failed to create transfer socket\n";
        return;
    }

    struct sockaddr_in addr;
    std::memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_port = htons(port);
    if (::inet_pton(AF_INET, "127.0.0.1", &addr.sin_addr) <= 0) {
        std::cerr << "[C++ Daemon] Invalid transfer address\n";
        closeSocket(sock_fd);
        return;
    }

    if (::connect(sock_fd, (struct sockaddr*)&addr, sizeof(addr)) < 0) {
        std::cerr << "[C++ Daemon] Failed to connect to port " << port << "\n";
        closeSocket(sock_fd);
        return;
    }

    struct timeval sock_timeout;
    sock_timeout.tv_sec = 30;
    sock_timeout.tv_usec = 0;
    ::setsockopt(sock_fd, SOL_SOCKET, SO_RCVTIMEO, &sock_timeout, sizeof(sock_timeout));
    ::setsockopt(sock_fd, SOL_SOCKET, SO_SNDTIMEO, &sock_timeout, sizeof(sock_timeout));

    if (expected_size <= 0) {
        std::cerr << "[C++ Daemon] Invalid expected_size " << expected_size << "\n";
        closeSocket(sock_fd);
        return;
    }

    fs::path dest_path = fs::path(watch_path) / rel_path;

    if (direction == "download") {
        fs::path tmp_path = dest_path.string() + ".tmp";
        try {
            fs::create_directories(dest_path.parent_path());

            std::ofstream outfile(tmp_path, std::ios::binary);
            if (!outfile) {
                std::cerr << "[C++ Daemon] Failed to open temp file: " << tmp_path << "\n";
                closeSocket(sock_fd);
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
                        std::cerr << "[C++ Daemon] Socket read timeout\n";
                    } else {
                        std::cerr << "[C++ Daemon] Socket read error\n";
                    }
                    break;
                }
                if (n == 0) break;

                outfile.write(buffer, n);
                if (outfile.fail()) {
                    std::cerr << "[C++ Daemon] Write failed\n";
                    break;
                }
                total_received += n;
            }

            outfile.close();

            if (total_received != expected_size) {
                std::cerr << "[C++ Daemon] Size mismatch: got " << total_received
                          << " expected " << expected_size << "\n";
                fs::remove(tmp_path);
                closeSocket(sock_fd);
                return;
            }

            if (!expected_hash.empty()) {
                std::string actual_hash;
                if (crypto::sha256_file(tmp_path.string(), actual_hash) &&
                    actual_hash != expected_hash) {
                    std::cerr << "[C++ Daemon] Hash mismatch\n";
                    fs::remove(tmp_path);
                    closeSocket(sock_fd);
                    return;
                }
            }

            fs::rename(tmp_path, dest_path);

            if (mode > 0) {
                try {
                    fs::permissions(dest_path, static_cast<fs::perms>(mode));
                } catch (const std::exception& e) {
                    std::cerr << "[C++ Daemon] Permissions error: " << e.what() << "\n";
                }
            }

            std::cout << "[C++ Daemon] Download complete: " << dest_path << "\n";
        } catch (const std::exception& e) {
            std::cerr << "[C++ Daemon] Download exception: " << e.what() << "\n";
            if (fs::exists(tmp_path)) fs::remove(tmp_path);
        }
    } else if (direction == "upload") {
        try {
            if (!fs::exists(dest_path)) {
                std::cerr << "[C++ Daemon] Upload file not found: " << dest_path << "\n";
                closeSocket(sock_fd);
                return;
            }

            std::ifstream infile(dest_path, std::ios::binary);
            if (!infile) {
                std::cerr << "[C++ Daemon] Failed to open: " << dest_path << "\n";
                closeSocket(sock_fd);
                return;
            }

            char buffer[4096];
            long long total_sent = 0;
            while (total_sent < expected_size) {
                long long to_read = std::min(4096LL, expected_size - total_sent);
                infile.read(buffer, to_read);
                ssize_t bytes = static_cast<ssize_t>(infile.gcount());
                if (bytes <= 0) break;

                ssize_t written = 0;
                while (written < bytes) {
                    ssize_t n = ::write(sock_fd, buffer + written, bytes - written);
                    if (n < 0) {
                        if (errno == EAGAIN || errno == EWOULDBLOCK) {
                            std::cerr << "[C++ Daemon] Socket write timeout\n";
                        } else {
                            std::cerr << "[C++ Daemon] Socket write error\n";
                        }
                        break;
                    }
                    written += n;
                }
                if (written < bytes) break;
                total_sent += written;
            }

            infile.close();
            std::cout << "[C++ Daemon] Upload complete: sent " << total_sent << " bytes\n";
        } catch (const std::exception& e) {
            std::cerr << "[C++ Daemon] Upload exception: " << e.what() << "\n";
        }
    }

    closeSocket(sock_fd);
}

void handle_chunked_file_download(
    const std::string &watch_path,
    const std::string &rel_path,
    const std::string &chunk_data,
    long long offset,
    const std::string &chunk_checksum,
    bool is_last,
    const std::string &expected_hash
) {
    fs::path dest_path = fs::path(watch_path) / rel_path;
    fs::path tmp_path = dest_path.string() + ".tmp";

    std::string actual_chunk_hash;
    {
        std::string tmp_hash_path = tmp_path.string() + ".chk";
        std::ofstream chk(tmp_hash_path, std::ios::binary);
        chk.write(chunk_data.data(), chunk_data.size());
        chk.close();
        crypto::sha256_file(tmp_hash_path, actual_chunk_hash);
        fs::remove(tmp_hash_path);
    }
    if (actual_chunk_hash != chunk_checksum) {
        std::cerr << "[C++ Daemon] Chunk checksum mismatch at offset " << offset
                  << " (got " << actual_chunk_hash << ", expected " << chunk_checksum << ")\n";
        return;
    }

    try {
        fs::create_directories(dest_path.parent_path());
        std::ofstream outfile(tmp_path, std::ios::binary | std::ios::in | std::ios::out);
        if (!outfile) {
            outfile.open(tmp_path, std::ios::binary);
        }
        if (!outfile) {
            std::cerr << "[C++ Daemon] Error: Failed to open temp file: " << tmp_path << "\n";
            return;
        }
        outfile.seekp(offset);
        outfile.write(chunk_data.data(), chunk_data.size());
        if (outfile.fail()) {
            std::cerr << "[C++ Daemon] Error: Failed to write chunk at offset " << offset << "\n";
            outfile.close();
            return;
        }
        outfile.close();

        if (is_last) {
            if (!expected_hash.empty()) {
                std::string actual_hash;
                if (crypto::sha256_file(tmp_path.string(), actual_hash) && actual_hash != expected_hash) {
                    std::cerr << "[C++ Daemon] Error: Full file hash mismatch for " << rel_path
                              << " (got " << actual_hash << ", expected " << expected_hash << ")\n";
                    fs::remove(tmp_path);
                    return;
                }
            }
            fs::rename(tmp_path, dest_path);
            std::cout << "[C++ Daemon] Chunked download completed atomically: " << dest_path << "\n";
        }
    } catch (const std::exception &e) {
        std::cerr << "[C++ Daemon] Exception during chunked download: " << e.what() << "\n";
    }
}

void handle_chunked_file_upload(
    const std::string &watch_path,
    const std::string &rel_path,
    int sock_fd,
    long long file_size,
    long long start_offset
) {
    fs::path src_path = fs::path(watch_path) / rel_path;
    std::ifstream infile(src_path, std::ios::binary);
    if (!infile) {
        std::cerr << "[C++ Daemon] Error: Failed to open file for chunked upload: " << src_path << "\n";
        return;
    }
    infile.seekg(start_offset);
    const long long CHUNK_SIZE = 16LL * 1024 * 1024;
    std::vector<char> buffer(CHUNK_SIZE);
    long long total_sent = start_offset;

    while (total_sent < file_size) {
        long long to_read = std::min(CHUNK_SIZE, file_size - total_sent);
        infile.read(buffer.data(), to_read);
        ssize_t bytes_read = infile.gcount();
        if (bytes_read <= 0) break;

        ssize_t written = 0;
        while (written < bytes_read) {
            ssize_t n = ::write(sock_fd, buffer.data() + written, bytes_read - written);
            if (n < 0) {
                std::cerr << "[C++ Daemon] Error: Socket write error during chunked upload: " << errno << "\n";
                infile.close();
                return;
            }
            written += n;
        }
        total_sent += written;
    }

    infile.close();
    std::cout << "[C++ Daemon] Chunked upload completed. Sent " << total_sent << " bytes for " << rel_path << "\n";
}

} // namespace transfer
