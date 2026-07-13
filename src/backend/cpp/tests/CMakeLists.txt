find_package(GTest QUIET)
if(NOT GTest_FOUND)
    include(FetchContent)
    FetchContent_Declare(googletest
        URL https://github.com/google/googletest/archive/refs/tags/v1.15.2.tar.gz)
    FetchContent_MakeAvailable(googletest)
    include(GoogleTest)
    if(NOT TARGET GTest::GTest)
        add_library(GTest::GTest ALIAS gtest)
    endif()
    if(NOT TARGET GTest::Main)
        add_library(GTest::Main ALIAS gtest_main)
    endif()
endif()

find_package(OpenSSL REQUIRED)

add_executable(test_ipc_client test_ipc_client.cpp ${CMAKE_SOURCE_DIR}/src/ipc_client.cpp)
target_link_libraries(test_ipc_client PRIVATE GTest::GTest GTest::Main)
target_include_directories(test_ipc_client PRIVATE ${CMAKE_SOURCE_DIR}/include)
gtest_discover_tests(test_ipc_client)

add_executable(test_sha256 test_sha256.cpp ${CMAKE_SOURCE_DIR}/src/sha256.cpp)
target_link_libraries(test_sha256 PRIVATE GTest::GTest GTest::Main ${OPENSSL_LIBRARIES})
target_include_directories(test_sha256 PRIVATE ${CMAKE_SOURCE_DIR}/include ${OPENSSL_INCLUDE_DIR})
gtest_discover_tests(test_sha256)

# Propagate the same sanitizer flags used by p2p_daemon (see top-level
# CMakeLists.txt) so sanitizer builds also cover the test binaries.
if(ENABLE_SANITIZERS AND NOT MSVC)
    foreach(p2p_test_target test_ipc_client test_sha256)
        target_compile_options(${p2p_test_target} PRIVATE ${P2P_SANITIZER_FLAGS})
        target_link_options(${p2p_test_target} PRIVATE ${P2P_SANITIZER_FLAGS})
    endforeach()
endif()
