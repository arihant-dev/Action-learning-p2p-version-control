find_package(GTest QUIET)
if(NOT GTest_FOUND)
    include(FetchContent)
    FetchContent_Declare(googletest
        URL https://github.com/google/googletest/archive/refs/tags/v1.15.2.tar.gz)
    FetchContent_MakeAvailable(googletest)
    include(GoogleTest)
endif()

add_executable(test_ipc_client test_ipc_client.cpp)
target_link_libraries(test_ipc_client PRIVATE GTest::GTest GTest::Main)
target_include_directories(test_ipc_client PRIVATE ${CMAKE_SOURCE_DIR}/include)
gtest_discover_tests(test_ipc_client)

add_executable(test_sha256 test_sha256.cpp)
target_link_libraries(test_sha256 PRIVATE GTest::GTest GTest::Main)
target_include_directories(test_sha256 PRIVATE ${CMAKE_SOURCE_DIR}/include)
gtest_discover_tests(test_sha256)
