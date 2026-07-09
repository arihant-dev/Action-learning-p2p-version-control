#include "filesystem_watcher.h"

#ifdef __APPLE__

#include <CoreServices/CoreServices.h>
#include <dispatch/dispatch.h>
#include <iostream>
#include <thread>
#include <string>
#include <filesystem>

namespace fs = std::filesystem;

class FSEventsWatcher : public FileSystemWatcher {
public:
    using FileSystemWatcher::FileSystemWatcher;
    ~FSEventsWatcher() override { stop(); }

    bool start() override {
        if (running_) return false;
        running_ = true;
        std::thread(&FSEventsWatcher::run, this).detach();
        return true;
    }

    void stop() override {
        running_ = false;
        if (stream_) {
            FSEventStreamStop(stream_);
            FSEventStreamInvalidate(stream_);
            FSEventStreamRelease(stream_);
            stream_ = nullptr;
        }
    }

private:
    FSEventStreamRef stream_ = nullptr;

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
        stream_ = FSEventStreamCreate(
            nullptr, &callback, &ctx, pathsToWatch,
            kFSEventStreamEventIdSinceNow, latency,
            kFSEventStreamCreateFlagFileEvents |
            kFSEventStreamCreateFlagWatchRoot);

        if (stream_) {
            dispatch_queue_t queue = dispatch_queue_create("p2p.fsevents", nullptr);
            FSEventStreamSetDispatchQueue(stream_, queue);
            FSEventStreamStart(stream_);
            dispatch_semaphore_t sem = dispatch_semaphore_create(0);
            dispatch_semaphore_wait(sem, DISPATCH_TIME_FOREVER);
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
