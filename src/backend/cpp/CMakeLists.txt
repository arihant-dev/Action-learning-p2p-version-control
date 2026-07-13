cmake_minimum_required(VERSION 3.16)
project(p2p-daemon LANGUAGES CXX)
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

if(WIN32)
    set(PLATFORM_SOURCES src/readdirectorychanges_watcher.cpp src/ipc_client_win.cpp)
    set(PLATFORM_LIBS ws2_32)
    add_definitions(-D_WIN32_WINNT=0x0601)
elseif(APPLE)
    set(PLATFORM_SOURCES src/fsevents_watcher.cpp)
    set(PLATFORM_LIBS "-framework CoreServices")
else()
    set(PLATFORM_SOURCES src/inotify_watcher.cpp)
    set(PLATFORM_LIBS "")
endif()

find_package(OpenSSL REQUIRED)

add_executable(p2p_daemon
    src/main.cpp
    src/ipc_client.cpp
    src/file_transfer.cpp
    src/filesystem_watcher.cpp
    src/sha256.cpp
    ${PLATFORM_SOURCES}
)

target_include_directories(p2p_daemon PRIVATE
    ${CMAKE_CURRENT_SOURCE_DIR}/include
    ${OPENSSL_INCLUDE_DIR}
)

target_link_libraries(p2p_daemon PRIVATE
    ${OPENSSL_LIBRARIES}
    ${PLATFORM_LIBS}
)

if(MSVC)
    target_compile_options(p2p_daemon PRIVATE /W4)
else()
    target_compile_options(p2p_daemon PRIVATE -Wall -Wextra -Wpedantic)
endif()

set_target_properties(p2p_daemon PROPERTIES
    RUNTIME_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}/bin"
)

# AddressSanitizer + UndefinedBehaviorSanitizer, off by default so normal
# (release/dev) builds are unaffected. Not supported by MSVC's toolchain here.
option(ENABLE_SANITIZERS "Build with AddressSanitizer + UBSan" OFF)
if(ENABLE_SANITIZERS AND NOT MSVC)
    set(P2P_SANITIZER_FLAGS -fsanitize=address,undefined -fno-omit-frame-pointer)
    target_compile_options(p2p_daemon PRIVATE ${P2P_SANITIZER_FLAGS})
    target_link_options(p2p_daemon PRIVATE ${P2P_SANITIZER_FLAGS})
elseif(ENABLE_SANITIZERS AND MSVC)
    message(WARNING "ENABLE_SANITIZERS is not supported with MSVC; ignoring.")
endif()

option(BUILD_TESTS "Build tests" OFF)
if(BUILD_TESTS)
    enable_testing()
    add_subdirectory(tests)
endif()
