#include "filesystem_watcher.h"
#include <iostream>
#include <map>
#include <set>
#include <filesystem>
#include <thread>

namespace fs = std::filesystem;

FileSystemWatcher::FileSystemWatcher(const std::string& watchPath, Callback callback)
    : watchPath_(watchPath), callback_(std::move(callback)) {}

void FileSystemWatcher::stop() {
    running_ = false;
}

class PollingWatcher : public FileSystemWatcher {
public:
    PollingWatcher(const std::string& watchPath, Callback callback,
                   std::chrono::milliseconds pollInterval)
        : FileSystemWatcher(watchPath, std::move(callback))
        , pollInterval_(pollInterval) {}

    bool start() override {
        if (running_) return false;
        running_ = true;
        std::thread(&PollingWatcher::watchLoop, this).detach();
        return true;
    }

private:
    std::chrono::milliseconds pollInterval_;

    void watchLoop() {
        struct FileInfo {
            fs::file_time_type lastWriteTime;
            uintmax_t size;
        };
        std::map<std::string, FileInfo> knownFiles;

        while (running_) {
            if (!fs::exists(watchPath_)) {
                std::this_thread::sleep_for(pollInterval_);
                continue;
            }

            std::set<std::string> seenFiles;
            try {
                for (const auto& entry : fs::recursive_directory_iterator(watchPath_)) {
                    if (!running_) return;
                    if (entry.is_symlink() || !entry.is_regular_file()) continue;

                    std::string relPath = fs::relative(entry.path(), watchPath_).generic_string();
                    if (relPath.empty() || relPath[0] == '.') continue;

                    auto lwt = fs::last_write_time(entry);
                    auto sz = entry.file_size();
                    seenFiles.insert(relPath);

                    auto it = knownFiles.find(relPath);
                    if (it == knownFiles.end()) {
                        knownFiles[relPath] = {lwt, sz};
                        if (callback_) {
                            WatchEvent ev{WatchEventType::Created, relPath, ""};
                            callback_(ev);
                        }
                    } else if (it->second.lastWriteTime != lwt || it->second.size != sz) {
                        it->second = {lwt, sz};
                        if (callback_) {
                            WatchEvent ev{WatchEventType::Modified, relPath, ""};
                            callback_(ev);
                        }
                    }
                }
            } catch (const std::exception& e) {
                std::cerr << "[FileSystemWatcher] Scan error: " << e.what() << "\n";
            }

            for (auto it = knownFiles.begin(); it != knownFiles.end();) {
                if (seenFiles.find(it->first) == seenFiles.end()) {
                    if (callback_) {
                        WatchEvent ev{WatchEventType::Deleted, it->first, ""};
                        callback_(ev);
                    }
                    it = knownFiles.erase(it);
                } else {
                    ++it;
                }
            }

            std::this_thread::sleep_for(pollInterval_);
        }
    }
};

#if !defined(__linux__) && !defined(__APPLE__) && !defined(_WIN32)
std::unique_ptr<FileSystemWatcher> createWatcher(
    const std::string& watchPath,
    FileSystemWatcher::Callback callback,
    std::chrono::milliseconds pollInterval)
{
    return std::make_unique<PollingWatcher>(watchPath, std::move(callback), pollInterval);
}
#endif
