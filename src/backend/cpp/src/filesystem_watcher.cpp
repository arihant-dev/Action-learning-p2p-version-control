#include "filesystem_watcher.h"

#include <iostream>
#include <set>
#include <chrono>

namespace fs = std::filesystem;

FileSystemWatcher::FileSystemWatcher(const std::string &watch_path, std::atomic<bool> &shutdown)
    : watch_path_(watch_path), shutdown_(shutdown) {}

FileSystemWatcher::~FileSystemWatcher() {
    stop();
}

bool FileSystemWatcher::start(FileChangeCallback callback) {
    callback_ = callback;

    try {
        if (!fs::exists(watch_path_)) {
            std::cerr << "[FileSystemWatcher] Error: Path does not exist: " << watch_path_ << "\n";
            return false;
        }

        // 1. Initial scan to populate the known state and notify Go coordinator
        scan_directory(true);
        std::cout << "[FileSystemWatcher] Initial baseline scan complete. Watching: " << watch_path_ << "\n";

        // 2. Start the polling thread
        watch_thread_ = std::thread(&FileSystemWatcher::watch_loop, this);
        return true;
    } catch (const std::exception &e) {
        std::cerr << "[FileSystemWatcher] Exception starting watcher: " << e.what() << "\n";
        return false;
    }
}

void FileSystemWatcher::stop() {
    shutdown_ = true;
    if (watch_thread_.joinable()) {
        watch_thread_.join();
    }
    std::cout << "[FileSystemWatcher] Stopped watching\n";
}

void FileSystemWatcher::watch_loop() {
    while (!shutdown_) {
        std::this_thread::sleep_for(std::chrono::milliseconds(1000)); // Poll every 1 second
        if (shutdown_) break;

        try {
            scan_directory(true);
        } catch (const std::exception &e) {
            std::cerr << "[FileSystemWatcher] Error during directory scan: " << e.what() << "\n";
        }
    }
}

void FileSystemWatcher::scan_directory(bool notify) {
    if (!fs::exists(watch_path_)) {
        return;
    }

    std::set<std::string> seen_files;

    for (const auto &entry : fs::recursive_directory_iterator(watch_path_)) {
        if (shutdown_) return;

        // Skip symlinks to prevent scanning files outside the watch directory
        if (entry.is_symlink()) {
            continue;
        }

        // Skip directories, only track regular files
        if (!entry.is_regular_file()) {
            continue;
        }

        try {
            std::string abs_path = entry.path().string();
            // Get relative path from watch_path_
            std::string rel_path = fs::relative(entry.path(), watch_path_).string();

            // Skip hidden files, dot-directories, python virtualenvs, node_modules, and build targets
            if (rel_path.empty() || 
                rel_path[0] == '.' || 
                rel_path.find("/.") != std::string::npos || 
                rel_path.find(".venv") != std::string::npos || 
                rel_path.find("venv/") != std::string::npos || 
                rel_path.find("node_modules/") != std::string::npos || 
                rel_path.find("target/") != std::string::npos) {
                continue;
            }

            auto last_write = fs::last_write_time(entry);
            auto size = entry.file_size();

            seen_files.insert(rel_path);

            auto it = known_files_.find(rel_path);
            if (it == known_files_.end()) {
                // New file detected
                known_files_[rel_path] = {last_write, size};
                if (notify && callback_) {
                    callback_(rel_path, "add");
                }
            } else {
                // Check if file was modified
                if (it->second.last_write_time != last_write || it->second.size != size) {
                    it->second.last_write_time = last_write;
                    it->second.size = size;
                    if (notify && callback_) {
                        callback_(rel_path, "modify");
                    }
                }
            }
        } catch (const std::exception &e) {
            // Log scan error for individual files (e.g. permission denied) and continue
            std::cerr << "[FileSystemWatcher] Warning: Failed to scan entry: " << entry.path() << " (" << e.what() << ")\n";
        }
    }

    // Check for deleted files
    for (auto it = known_files_.begin(); it != known_files_.end();) {
        if (shutdown_) return;

        if (seen_files.find(it->first) == seen_files.end()) {
            // File deleted
            std::string deleted_path = it->first;
            it = known_files_.erase(it); // Erase first to prevent callback accessing stale entry
            if (notify && callback_) {
                callback_(deleted_path, "delete");
            }
        } else {
            ++it;
        }
    }
}
