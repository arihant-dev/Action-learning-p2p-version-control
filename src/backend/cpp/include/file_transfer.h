#pragma once

#include <string>

namespace transfer {

void handle_file_transfer(
    const std::string &watch_path,
    const std::string &rel_path,
    int port,
    const std::string &direction,
    long long expected_size,
    const std::string &expected_hash
);

} // namespace transfer
