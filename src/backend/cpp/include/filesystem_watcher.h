#pragma once

#include <functional>
#include <string>
#include <thread>
#include <atomic>
#include <map>
#include <filesystem>

class FileSystemWatcher {
public:
    using FileChangeCallback =
        std::function<void(const std::string &path, const std::string &action)>;

    FileSystemWatcher(const std::string &watch_path, std::atomic<bool> &shutdown);
    ~FileSystemWatcher();

    bool start(FileChangeCallback callback);
    void stop();

private:
    const std::string watch_path_;
    std::atomic<bool> &shutdown_;
    std::thread watch_thread_;
    FileChangeCallback callback_;

    struct FileInfo {
        std::filesystem::file_time_type last_write_time;
        uintmax_t size;
    };
    std::map<std::string, FileInfo> known_files_;

    void watch_loop();
    void scan_directory(bool notify);
};
