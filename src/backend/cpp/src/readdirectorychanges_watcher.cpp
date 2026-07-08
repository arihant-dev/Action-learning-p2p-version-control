#include "filesystem_watcher.h"

#ifdef _WIN32

#include <windows.h>
#include <iostream>
#include <thread>
#include <string>
#include <vector>
#include <filesystem>

namespace fs = std::filesystem;

class ReadDirectoryChangesWatcher : public FileSystemWatcher {
public:
    using FileSystemWatcher::FileSystemWatcher;
    ~ReadDirectoryChangesWatcher() override { stop(); }

    bool start() override {
        if (running_) return false;
        running_ = true;
        std::thread(&ReadDirectoryChangesWatcher::watchLoop, this).detach();
        return true;
    }

    void stop() override {
        running_ = false;
        if (hDir_ != INVALID_HANDLE_VALUE) {
            CancelIoEx(hDir_, nullptr);
            CloseHandle(hDir_);
            hDir_ = INVALID_HANDLE_VALUE;
        }
    }

private:
    HANDLE hDir_ = INVALID_HANDLE_VALUE;

    void watchLoop() {
        hDir_ = CreateFileW(
            std::wstring(watchPath_.begin(), watchPath_.end()).c_str(),
            FILE_LIST_DIRECTORY,
            FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE,
            nullptr,
            OPEN_EXISTING,
            FILE_FLAG_BACKUP_SEMANTICS | FILE_FLAG_OVERLAPPED,
            nullptr);

        if (hDir_ == INVALID_HANDLE_VALUE) {
            std::cerr << "[ReadDirectoryChangesWatcher] Failed to open directory\n";
            return;
        }

        std::vector<BYTE> buffer(65536);
        OVERLAPPED overlapped{};
        overlapped.hEvent = CreateEvent(nullptr, TRUE, FALSE, nullptr);

        while (running_) {
            DWORD bytesReturned = 0;
            BOOL success = ReadDirectoryChangesW(
                hDir_, buffer.data(), static_cast<DWORD>(buffer.size()),
                TRUE,
                FILE_NOTIFY_CHANGE_FILE_NAME |
                FILE_NOTIFY_CHANGE_DIR_NAME |
                FILE_NOTIFY_CHANGE_LAST_WRITE |
                FILE_NOTIFY_CHANGE_SIZE |
                FILE_NOTIFY_CHANGE_CREATION,
                &bytesReturned, &overlapped, nullptr);

            if (!success) {
                std::cerr << "[ReadDirectoryChangesWatcher] ReadDirectoryChangesW failed\n";
                break;
            }

            DWORD waitResult = WaitForSingleObject(overlapped.hEvent, 500);
            if (waitResult == WAIT_TIMEOUT) continue;
            if (waitResult != WAIT_OBJECT_0) break;

            DWORD dwBytes = 0;
            GetOverlappedResult(hDir_, &overlapped, &dwBytes, FALSE);

            auto* notify = reinterpret_cast<FILE_NOTIFY_INFORMATION*>(buffer.data());
            while (notify) {
                std::wstring wname(notify->FileName, notify->FileNameLength / sizeof(WCHAR));
                std::string name(wname.begin(), wname.end());

                switch (notify->Action) {
                    case FILE_ACTION_ADDED:
                        if (callback_) callback_({WatchEventType::Created, name, ""});
                        break;
                    case FILE_ACTION_MODIFIED:
                        if (callback_) callback_({WatchEventType::Modified, name, ""});
                        break;
                    case FILE_ACTION_REMOVED:
                        if (callback_) callback_({WatchEventType::Deleted, name, ""});
                        break;
                    case FILE_ACTION_RENAMED_OLD_NAME:
                        if (callback_) callback_({WatchEventType::Deleted, name, ""});
                        break;
                    case FILE_ACTION_RENAMED_NEW_NAME:
                        if (callback_) callback_({WatchEventType::Created, name, ""});
                        break;
                }

                if (notify->NextEntryOffset == 0) break;
                notify = reinterpret_cast<FILE_NOTIFY_INFORMATION*>(
                    reinterpret_cast<BYTE*>(notify) + notify->NextEntryOffset);
            }

            ResetEvent(overlapped.hEvent);
        }

        if (overlapped.hEvent) CloseHandle(overlapped.hEvent);
        if (hDir_ != INVALID_HANDLE_VALUE) CloseHandle(hDir_);
        hDir_ = INVALID_HANDLE_VALUE;
    }
};

std::unique_ptr<FileSystemWatcher> createWatcher(
    const std::string& watchPath,
    FileSystemWatcher::Callback callback,
    std::chrono::milliseconds pollInterval)
{
    (void)pollInterval;
    return std::make_unique<ReadDirectoryChangesWatcher>(watchPath, std::move(callback));
}

#endif
