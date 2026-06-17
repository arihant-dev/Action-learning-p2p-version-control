#include <functional>
#include <nlohmann/json.hpp>
#include <string>
#include <sys/inotify.h>
#include <thread>
#include <unistd.h>

// Linux-only file system watcher using inotify
class FileSystemWatcher {
public:
  using FileChangeCallback =
      std::function<void(const std::string &, const std::string &)>;
  FileSystemWatcher(const std::string &watch_path, std::atomic<bool> &shutdown)
      : watch_path_(watch_path), shutdown_(shutdown), inotify_fd_(-1),
        watch_descriptor_(-1) {}
  ~FileSystemWatcher();
  bool start(FileChangeCallback callback);
  void stop();

private:
  const std::string watch_path_;
  std::atomic<bool> &shutdown_;
  int inotify_fd_;
  int watch_descriptor_;
  std::thread watch_thread_;
  FileChangeCallback callback_;
  void watch_loop();
};
