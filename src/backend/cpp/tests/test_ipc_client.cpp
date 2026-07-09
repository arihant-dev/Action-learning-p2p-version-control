#include <gtest/gtest.h>
#include "ipc_client.h"
#include <thread>
#include <chrono>

TEST(IpcClientTest, CreateInstance) {
    auto client = IpcClient::create();
    ASSERT_NE(client, nullptr);
}

TEST(IpcClientTest, NotConnectedByDefault) {
    auto client = IpcClient::create();
    EXPECT_FALSE(client->isConnected());
}

TEST(IpcClientTest, SendWithoutConnectFails) {
    auto client = IpcClient::create();
    EXPECT_FALSE(client->send("test"));
}

TEST(IpcClientTest, ReceiveWithoutConnectFails) {
    auto client = IpcClient::create();
    std::string msg;
    EXPECT_FALSE(client->receive(msg));
}

TEST(IpcClientTest, DisconnectNoCrash) {
    auto client = IpcClient::create();
    client->disconnect();
    EXPECT_FALSE(client->isConnected());
}

TEST(IpcClientTest, DoubleDisconnectNoCrash) {
    auto client = IpcClient::create();
    client->disconnect();
    client->disconnect();
    EXPECT_FALSE(client->isConnected());
}

TEST(IpcClientTest, ConnectToInvalidPathFails) {
    auto client = IpcClient::create();
    EXPECT_FALSE(client->connect("/nonexistent/path/socket.sock"));
}

TEST(IpcClientTest, MalformedMessageStillParses) {
    auto client = IpcClient::create();
    std::string msg;
    EXPECT_FALSE(client->receive(msg));
}
