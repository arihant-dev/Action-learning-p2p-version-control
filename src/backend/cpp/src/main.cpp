#include "filesystem_watcher.h"

#include <atomic>
#include <csignal>
#include <filesystem>
#include <format>
#include <iostream>
#include <nlohmann/json.hpp>
#include <string>
#include <sys/inotify.h>
#include <thread>
#include <unistd.h>

// Global shutdown flag

std::atomic<bool> g_shutdown(false);
// Signal handler
void signal_handler(int signal) {
  std::cout << std::format("Received signal: {}\n", signal);
  g_shutdown = true;
}

void print_usage(const char *program_name) {
  std::cout << std::format("Usage: {} <path>\n", program_name);
  std::cout << std::format("Example: {} /home/user/sync\n", program_name);
}

int main(int argc, char *argv[]) {
  try {
    // Parse command line arguments
    if (argc < 2) {
      print_usage(argv[0]);
      return 1;
    }

    std::string watch_path = argv[1];

    // Validate path exists
    // TODO:  If not exists, we create one
    if (!std::filesystem::exists(watch_path)) {
      std::cerr << std::format("Error: Path does not exist: {}\n", watch_path);
      return 1;
    }

    if (!std::filesystem::is_directory(watch_path)) {
      std::cerr << std::format("Error: Path is not a directory: {}\n",
                               watch_path);
      return 1;
    }

    std::cout << "P2P File Sync - C++ Daemon (Linux)\n";
    std::cout << std::format("Watching directory: {}\n", watch_path);
    std::cout << "Press Ctrl+C to stop\n\n";

    // Setup signal handlers
    signal(SIGINT, signal_handler);
    signal(SIGTERM, signal_handler);

    // Create file watcher
    FileSystemWatcher watcher(watch_path, g_shutdown);

    // Callback for file changes
    auto on_file_change = [](const std::string &filename,
                             const std::string &action) {
      nlohmann::json event;
      event["timestamp"] =
          std::chrono::system_clock::now().time_since_epoch().count();
      event["action"] = action;
      event["filename"] = filename;
      event["type"] = "file_changed";

      std::cout << std::format("[EVENT] File {}: {}\n", action, filename);
      std::cout << std::format("[JSON] {}\n", event.dump());
    };

    // Start watching
    if (!watcher.start(on_file_change)) {
      std::cerr << std::format("Error: Failed to start file watcher\n");
      return 1;
    }

    // Main loop - keep running until shutdown signal
    while (!g_shutdown) {
      std::this_thread::sleep_for(std::chrono::milliseconds(100));
    }

    // Cleanup
    watcher.stop();
    std::cout << "\nDone\n";

    return 0;
  } catch (const std::exception &e) {
    std::cerr << std::format("Error: {}\n", e.what());
    return 1;
  }
}
