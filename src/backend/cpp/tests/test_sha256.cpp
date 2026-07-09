#include <gtest/gtest.h>
#include "sha256.h"
#include <fstream>
#include <cstdio>

class Sha256Test : public ::testing::Test {
protected:
    void SetUp() override {
        emptyFile = "test_empty.tmp";
        smallFile = "test_small.tmp";
        largeFile = "test_large.tmp";

        std::ofstream(emptyFile).close();

        std::ofstream smallOut(smallFile);
        smallOut << "Hello, World!";
        smallOut.close();

        std::ofstream largeOut(largeFile);
        for (int i = 0; i < 10000; ++i) {
            largeOut << "Line " << i << " of test content for hashing.\n";
        }
        largeOut.close();
    }

    void TearDown() override {
        std::remove(emptyFile.c_str());
        std::remove(smallFile.c_str());
        std::remove(largeFile.c_str());
    }

    std::string emptyFile;
    std::string smallFile;
    std::string largeFile;
};

TEST_F(Sha256Test, KnownVectors) {
    EXPECT_EQ(crypto::sha256(""),
              "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
    EXPECT_EQ(crypto::sha256("abc"),
              "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
    EXPECT_EQ(crypto::sha256("message digest"),
              "f7846f55cf23e14eebeab5b4e1550cad5b509e3348fbc4efa3a1413d393cb650");
}

TEST_F(Sha256Test, EmptyString) {
    std::string hash = crypto::sha256("");
    EXPECT_EQ(hash.size(), 64ULL);
    EXPECT_EQ(hash, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
}

TEST_F(Sha256Test, NonEmptyString) {
    std::string hash = crypto::sha256("Hello, World!");
    EXPECT_EQ(hash.size(), 64ULL);
}

TEST_F(Sha256Test, EmptyFile) {
    std::string hash;
    EXPECT_TRUE(crypto::sha256_file(emptyFile, hash));
    EXPECT_EQ(hash.size(), 64ULL);
    EXPECT_EQ(hash, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
}

TEST_F(Sha256Test, SmallFile) {
    std::string hash;
    EXPECT_TRUE(crypto::sha256_file(smallFile, hash));
    EXPECT_EQ(hash.size(), 64ULL);
}

TEST_F(Sha256Test, LargeFile) {
    std::string hash;
    EXPECT_TRUE(crypto::sha256_file(largeFile, hash));
    EXPECT_EQ(hash.size(), 64ULL);
}

TEST_F(Sha256Test, NonexistentFile) {
    std::string hash;
    EXPECT_FALSE(crypto::sha256_file("/nonexistent/file/path", hash));
}

TEST_F(Sha256Test, ConsistentResults) {
    std::string h1 = crypto::sha256("test data");
    std::string h2 = crypto::sha256("test data");
    EXPECT_EQ(h1, h2);
}

TEST_F(Sha256Test, DifferentInputsDifferentHashes) {
    std::string h1 = crypto::sha256("input one");
    std::string h2 = crypto::sha256("input two");
    EXPECT_NE(h1, h2);
}
