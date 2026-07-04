cmake_minimum_required(VERSION 3.10)
project(p2p_cpp_daemon)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

find_package(Python3 REQUIRED COMPONENTS Interpreter)

# Main executable - Cross-Platform
add_executable(cpp_daemon
    src/main.cpp
    src/filesystem_watcher.cpp
    src/ipc_client.cpp
    src/sha256.cpp
    src/file_transfer.cpp
)

# Include directories
target_include_directories(cpp_daemon PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/include
)

target_link_libraries(cpp_daemon PRIVATE
    pthread
)

# Compiler warnings
target_compile_options(cpp_daemon PRIVATE -Wall -Wextra -Wpedantic)

# Build output
set_target_properties(cpp_daemon PROPERTIES
    RUNTIME_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}/bin"
)

message(STATUS "Building C++ daemon for Cross-Platform (std::filesystem)")
message(STATUS "Output: ${CMAKE_BINARY_DIR}/bin/cpp_daemon")


enable_testing()
add_test(
    NAME daemon_file_create_test
    COMMAND
        ${Python3_EXECUTABLE}
        ${CMAKE_CURRENT_SOURCE_DIR}/tests/test_daemon_file_create.py
        $<TARGET_FILE:cpp_daemon>
)
