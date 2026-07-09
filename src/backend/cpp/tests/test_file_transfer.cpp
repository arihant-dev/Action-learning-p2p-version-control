#include <gtest/gtest.h>
#include "file_transfer.h"
#include "sha256.h"
#include <filesystem>
#include <fstream>
#include <thread>
#include <chrono>

namespace fs = std::filesystem;

class FileTransferTest : public ::testing::Test {
protected:
    void SetUp() override {
        testDir = fs::temp_directory_path() / "p2p_transfer_test";
        fs::create_directories(testDir);
    }

    void TearDown() override {
        fs::remove_all(testDir);
    }

    fs::path testDir;
};

TEST(FileTransferTest, InvalidDirection) {
    fs::path dir = fs::temp_directory_path() / "p2p_transfer_invalid";
    fs::create_directories(dir);
    transfer::handle_file_transfer(
        dir.string(), "test.txt", 9999, "invalid_direction", 100, "", 0);
    fs::remove_all(dir);
}

TEST(FileTransferTest, InvalidPortConnectFails) {
    fs::path dir = fs::temp_directory_path() / "p2p_transfer_port";
    fs::create_directories(dir);
    transfer::handle_file_transfer(
        dir.string(), "test.txt", 1, "download", 100, "", 0);
    fs::remove_all(dir);
}

TEST(FileTransferTest, UploadNonexistentFile) {
    fs::path dir = fs::temp_directory_path() / "p2p_transfer_upload";
    fs::create_directories(dir);
    transfer::handle_file_transfer(
        dir.string(), "nonexistent.txt", 9998, "upload", 100, "", 0);
    fs::remove_all(dir);
}

TEST(FileTransferTest, NegativeExpectedSize) {
    fs::path dir = fs::temp_directory_path() / "p2p_transfer_neg";
    fs::create_directories(dir);
    std::ofstream(dir / "test.txt") << "content";
    transfer::handle_file_transfer(
        dir.string(), "test.txt", 9997, "download", -1, "", 0);
    fs::remove_all(dir);
}

TEST(FileTransferTest, ZeroExpectedSize) {
    fs::path dir = fs::temp_directory_path() / "p2p_transfer_zero";
    fs::create_directories(dir);
    std::ofstream(dir / "test.txt") << "content";
    transfer::handle_file_transfer(
        dir.string(), "test.txt", 9996, "download", 0, "", 0);
    fs::remove_all(dir);
}
