#include <gtest/gtest.h>
#include "filesystem_watcher.h"
#include <filesystem>
#include <fstream>
#include <thread>
#include <chrono>
#include <atomic>

namespace fs = std::filesystem;

class FileSystemWatcherTest : public ::testing::Test {
protected:
    void SetUp() override {
        testDir = fs::temp_directory_path() / "p2p_watcher_test";
        fs::create_directories(testDir);
    }

    void TearDown() override {
        fs::remove_all(testDir);
    }

    fs::path testDir;
};

TEST_F(FileSystemWatcherTest, CreateWatcherInstance) {
    auto watcher = createWatcher(testDir.string(), [](const WatchEvent&) {});
    ASSERT_NE(watcher, nullptr);
}

TEST_F(FileSystemWatcherTest, StartAndStop) {
    auto watcher = createWatcher(testDir.string(), [](const WatchEvent&) {});
    ASSERT_TRUE(watcher->start());
    EXPECT_TRUE(watcher->isRunning());
    watcher->stop();
    EXPECT_FALSE(watcher->isRunning());
}

TEST_F(FileSystemWatcherTest, DoubleStartFails) {
    auto watcher = createWatcher(testDir.string(), [](const WatchEvent&) {});
    ASSERT_TRUE(watcher->start());
    EXPECT_FALSE(watcher->start());
    watcher->stop();
}

TEST_F(FileSystemWatcherTest, CreateFileDetected) {
    std::atomic<bool> detected{false};
    auto watcher = createWatcher(testDir.string(),
        [&](const WatchEvent& event) {
            if (event.type == WatchEventType::Created) detected = true;
        },
        std::chrono::milliseconds(100));

    ASSERT_TRUE(watcher->start());
    std::this_thread::sleep_for(std::chrono::milliseconds(200));

    std::ofstream(testDir / "test.txt") << "hello";
    std::this_thread::sleep_for(std::chrono::milliseconds(500));

    EXPECT_TRUE(detected);
    watcher->stop();
}

TEST_F(FileSystemWatcherTest, ModifyFileDetected) {
    std::atomic<bool> modified{false};
    std::ofstream(testDir / "mod.txt") << "original";
    std::this_thread::sleep_for(std::chrono::milliseconds(100));

    auto watcher = createWatcher(testDir.string(),
        [&](const WatchEvent& event) {
            if (event.type == WatchEventType::Modified) modified = true;
        },
        std::chrono::milliseconds(100));

    ASSERT_TRUE(watcher->start());
    std::this_thread::sleep_for(std::chrono::milliseconds(200));

    std::ofstream(testDir / "mod.txt") << "modified content";
    std::this_thread::sleep_for(std::chrono::milliseconds(500));

    EXPECT_TRUE(modified);
    watcher->stop();
}

TEST_F(FileSystemWatcherTest, DeleteFileDetected) {
    std::atomic<bool> deleted{false};
    std::ofstream(testDir / "del.txt") << "to delete";
    std::this_thread::sleep_for(std::chrono::milliseconds(100));

    auto watcher = createWatcher(testDir.string(),
        [&](const WatchEvent& event) {
            if (event.type == WatchEventType::Deleted) deleted = true;
        },
        std::chrono::milliseconds(100));

    ASSERT_TRUE(watcher->start());
    std::this_thread::sleep_for(std::chrono::milliseconds(200));

    fs::remove(testDir / "del.txt");
    std::this_thread::sleep_for(std::chrono::milliseconds(500));

    EXPECT_TRUE(deleted);
    watcher->stop();
}
