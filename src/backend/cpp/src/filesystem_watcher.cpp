#include "filesystem_watcher.h"

#include <format>
#include <iostream>
// Global shutdown flag
// extern std::atomic<bool> g_shutdown;

FileSystemWatcher::~FileSystemWatcher() { stop(); }

bool FileSystemWatcher::start(FileSystemWatcher::FileChangeCallback callback) {

  callback_ = callback;

  // Create inotify instance
  // TODO: We need to also watch sub directory. We will check how to do that
  // later.
  inotify_fd_ = inotify_init1(IN_NONBLOCK);
  if (inotify_fd_ < 0) {
    std::cerr << std::format("Error: Failed to create inotify instance\n");
    return false;
  }

  // Add watch for directory
  watch_descriptor_ =
      inotify_add_watch(inotify_fd_, watch_path_.c_str(),
                        IN_CREATE | IN_MOVED_TO | IN_MODIFY | IN_DELETE);
  if (watch_descriptor_ < 0) {
    std::cerr << std::format("Error: Failed to add watch descriptor\n");
    close(inotify_fd_);
    inotify_fd_ = -1;
    return false;
  }

  std::cerr << std::format("[inotify] Watching: {}\n", watch_path_);

  // Start watch thread
  watch_thread_ = std::thread(&FileSystemWatcher::watch_loop, this);
  return true;
}

void FileSystemWatcher::stop() {
  shutdown_ = true;

  if (watch_thread_.joinable()) {
    watch_thread_.join();
  }

  if (watch_descriptor_ >= 0 && inotify_fd_ >= 0) {
    inotify_rm_watch(inotify_fd_, watch_descriptor_);
    watch_descriptor_ = -1;
  }

  if (inotify_fd_ >= 0) {
    close(inotify_fd_);
    inotify_fd_ = -1;
  }

  std::cerr << std::format("[inotify] Stopped watching\n");
}

void FileSystemWatcher::watch_loop() {
  const size_t BUF_LEN = 4096;
  char buf[BUF_LEN] __attribute__((aligned(__alignof__(struct inotify_event))));

  std::cerr << std::format("[inotify] Watch loop started\n");

  while (!shutdown_) {
    ssize_t len = read(inotify_fd_, buf, BUF_LEN);

    if (len <= 0) {
      std::this_thread::sleep_for(std::chrono::milliseconds(10));
      continue;
    }

    // Process events
    for (char *p = buf; p < buf + len;) {
      struct inotify_event *event = (struct inotify_event *)p;

      if (event->len > 0) {
        std::string filename = event->name;
        std::string action;

        // TODO: I don't like string-based comparison. We will change it to enum
        // later.
        if (event->mask & IN_CREATE) {
          action = "created";
        } else if (event->mask & IN_MOVED_TO) {
          action = "moved_to";
        } else if (event->mask & IN_DELETE) {
          action = "deleted";
        } else if (event->mask & IN_MODIFY) {
          action = "modified";
        }

        if (!action.empty() && callback_) {
          callback_(filename, action);
        }
      }

      p += sizeof(struct inotify_event) + event->len;
    }
  }

  std::cout << "[inotify] Watch loop ended\n";
}
