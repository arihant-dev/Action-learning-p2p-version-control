#include "filesystem_watcher.h"

#ifdef __APPLE__

#include <CoreServices/CoreServices.h>
#include <dispatch/dispatch.h>
#include <iostream>
#include <thread>
#include <mutex>
#include <string>
#include <filesystem>

namespace fs = std::filesystem;

class FSEventsWatcher : public FileSystemWatcher {
public:
    using FileSystemWatcher::FileSystemWatcher;
    ~FSEventsWatcher() override { stop(); }

    bool start() override {
        if (running_.exchange(true)) return false;
        thread_ = std::thread(&FSEventsWatcher::run, this);
        return true;
    }

    void stop() override {
        if (!running_.exchange(false)) return;
        {
            std::lock_guard<std::mutex> lock(streamMutex_);
            if (stream_) {
                FSEventStreamStop(stream_);
                FSEventStreamInvalidate(stream_);
                FSEventStreamRelease(stream_);
                stream_ = nullptr;
            }
        }
        if (sem_) {
            dispatch_semaphore_signal(sem_); // unblock run()
        }
        if (thread_.joinable()) {
            thread_.join();
        }
    }

private:
    std::thread thread_;
    std::mutex streamMutex_;
    FSEventStreamRef stream_ = nullptr;
    dispatch_semaphore_t sem_ = nullptr;
    dispatch_queue_t queue_ = nullptr;

    static void callback(
        ConstFSEventStreamRef,
        void* info,
        size_t numEvents,
        void* eventPaths,
        const FSEventStreamEventFlags eventFlags[],
        const FSEventStreamEventId[])
    {
        auto* self = static_cast<FSEventsWatcher*>(info);
        auto paths = static_cast<const char**>(eventPaths);
        for (size_t i = 0; i < numEvents; ++i) {
            std::string path(paths[i]);
            if (path.compare(0, self->watchPath_.size(), self->watchPath_) == 0) {
                if (path.size() > self->watchPath_.size() + 1) {
                    path = path.substr(self->watchPath_.size() + 1);
                } else {
                    path = "";
                }
            }
            if (eventFlags[i] & kFSEventStreamEventFlagItemCreated)
                if (self->callback_) self->callback_({WatchEventType::Created, path, ""});
            if (eventFlags[i] & kFSEventStreamEventFlagItemModified)
                if (self->callback_) self->callback_({WatchEventType::Modified, path, ""});
            if (eventFlags[i] & kFSEventStreamEventFlagItemRemoved)
                if (self->callback_) self->callback_({WatchEventType::Deleted, path, ""});
            if (eventFlags[i] & kFSEventStreamEventFlagItemRenamed)
                if (self->callback_) self->callback_({WatchEventType::Moved, path, ""});
        }
    }

    void run() {
        CFStringRef cfPath = CFStringCreateWithCString(
            nullptr, watchPath_.c_str(), kCFStringEncodingUTF8);
        CFArrayRef pathsToWatch = CFArrayCreate(
            nullptr, (const void**)&cfPath, 1, &kCFTypeArrayCallBacks);

        FSEventStreamContext ctx{};
        ctx.info = this;

        CFAbsoluteTime latency = 0.5;
        FSEventStreamRef s = FSEventStreamCreate(
            nullptr, &callback, &ctx, pathsToWatch,
            kFSEventStreamEventIdSinceNow, latency,
            kFSEventStreamCreateFlagFileEvents |
            kFSEventStreamCreateFlagWatchRoot);

        {
            std::lock_guard<std::mutex> lock(streamMutex_);
            if (!running_) {
                if (s) {
                    FSEventStreamRelease(s);
                }
                CFRelease(pathsToWatch);
                CFRelease(cfPath);
                return;
            }
            stream_ = s;
        }

        if (s) {
            queue_ = dispatch_queue_create("p2p.fsevents", nullptr);
            FSEventStreamSetDispatchQueue(s, queue_);
            FSEventStreamStart(s);
            sem_ = dispatch_semaphore_create(0);
            dispatch_semaphore_wait(sem_, DISPATCH_TIME_FOREVER);
            dispatch_release(queue_);
            queue_ = nullptr;
        }

        CFRelease(pathsToWatch);
        CFRelease(cfPath);
    }
};

std::unique_ptr<FileSystemWatcher> createWatcher(
    const std::string& watchPath,
    FileSystemWatcher::Callback callback,
    std::chrono::milliseconds pollInterval)
{
    (void)pollInterval;
    return std::make_unique<FSEventsWatcher>(watchPath, std::move(callback));
}

#endif
