cmake_minimum_required(VERSION 3.10)
project(p2p_cpp_daemon)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

find_package(nlohmann_json 3.12.0 REQUIRED)
include(FetchContent)

# Main executable - Linux only
add_executable(cpp_daemon
    src/main.cpp
    src/filesystem_watcher.cpp
)

# Include directories
target_include_directories(cpp_daemon PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/include
)

target_link_libraries(cpp_daemon PRIVATE
    nlohmann_json::nlohmann_json
    pthread
)


# Compiler warnings
target_compile_options(cpp_daemon PRIVATE -Wall -Wextra -Wpedantic)

# Build output
set_target_properties(cpp_daemon PROPERTIES
    RUNTIME_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}/bin"
)

message(STATUS "Building C++ daemon for Linux (inotify)")
message(STATUS "Output: ${CMAKE_BINARY_DIR}/bin/cpp_daemon")
