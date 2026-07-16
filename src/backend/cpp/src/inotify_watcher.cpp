#include "filesystem_watcher.h"

#ifdef __linux__

#include <sys/inotify.h>
#include <unistd.h>
#include <poll.h>
#include <iostream>
#include <map>
#include <cstring>
#include <filesystem>
#include <thread>

namespace fs = std::filesystem;

class InotifyWatcher : public FileSystemWatcher {
public:
    using FileSystemWatcher::FileSystemWatcher;
    ~InotifyWatcher() override { stop(); }

    bool start() override {
        if (running_) return false;
        inotifyFd_ = inotify_init1(IN_NONBLOCK);
        if (inotifyFd_ < 0) {
            std::cerr << "[InotifyWatcher] inotify_init1 failed\n";
            return false;
        }
        addWatchRecursive(watchPath_);
        running_ = true;
        std::thread(&InotifyWatcher::handleEvents, this).detach();
        return true;
    }

    void stop() override {
        running_ = false;
        if (inotifyFd_ >= 0) {
            ::close(inotifyFd_);
            inotifyFd_ = -1;
        }
    }

private:
    int inotifyFd_ = -1;
    std::map<int, std::string> wdToPath_; // watch descriptor -> relative directory path

    std::string relativeDir(const std::string& path) {
        if (path == watchPath_) {
            return "";
        }
        if (path.size() > watchPath_.size() &&
            path.compare(0, watchPath_.size(), watchPath_) == 0 &&
            path[watchPath_.size()] == '/') {
            std::string rel = path.substr(watchPath_.size() + 1);
            return rel;
        }
        return "";
    }

    void addWatchRecursive(const std::string& path) {
        uint32_t mask = IN_CREATE | IN_MODIFY | IN_DELETE | IN_MOVED_FROM | IN_MOVED_TO;
        int wd = inotify_add_watch(inotifyFd_, path.c_str(), mask);
        if (wd < 0) {
            std::cerr << "[InotifyWatcher] Failed to watch " << path << "\n";
            return;
        }
        wdToPath_[wd] = relativeDir(path);
        try {
            for (const auto& entry : fs::directory_iterator(path)) {
                if (entry.is_directory()) {
                    addWatchRecursive(entry.path().string());
                }
            }
        } catch (...) {}
    }

    void handleEvents() {
        static const size_t BUF_LEN = 4096;
        alignas(inotify_event) char buf[BUF_LEN];

        while (running_) {
            struct pollfd pfd = { inotifyFd_, POLLIN, 0 };
            int ret = poll(&pfd, 1, 500);
            if (ret < 0) break;
            if (ret == 0) continue;

            ssize_t len = read(inotifyFd_, buf, BUF_LEN);
            if (len <= 0) continue;

            for (char* ptr = buf; ptr < buf + len; ) {
                auto* event = reinterpret_cast<const inotify_event*>(ptr);
                if (event->len > 0) {
                    std::string name(event->name);
                    std::string relPath = name;
                    auto it = wdToPath_.find(event->wd);
                    if (it != wdToPath_.end() && !it->second.empty()) {
                        relPath = it->second + "/" + name;
                    }

                    if (event->mask & IN_CREATE)
                        if (callback_) callback_({WatchEventType::Created, relPath, ""});
                    if (event->mask & IN_MODIFY)
                        if (callback_) callback_({WatchEventType::Modified, relPath, ""});
                    if (event->mask & IN_DELETE)
                        if (callback_) callback_({WatchEventType::Deleted, relPath, ""});
                    if (event->mask & IN_MOVED_FROM)
                        if (callback_) callback_({WatchEventType::Deleted, relPath, ""});
                    if (event->mask & IN_MOVED_TO)
                        if (callback_) callback_({WatchEventType::Created, relPath, ""});
                }
                ptr += sizeof(inotify_event) + event->len;
            }
        }
    }
};

std::unique_ptr<FileSystemWatcher> createWatcher(
    const std::string& watchPath,
    FileSystemWatcher::Callback callback,
    std::chrono::milliseconds pollInterval)
{
    (void)pollInterval;
    return std::make_unique<InotifyWatcher>(watchPath, std::move(callback));
}

#endif
