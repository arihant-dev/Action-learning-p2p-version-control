cmake_minimum_required(VERSION 3.20)
project(p2p_backend_cpp LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

add_executable(p2p_backend_cpp src/main.cpp)
target_include_directories(p2p_backend_cpp PRIVATE include)

