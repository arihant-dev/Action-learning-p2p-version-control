#pragma once

#include <string>

namespace crypto {

std::string sha256(const std::string &input);
std::string sha256_file(const std::string &path);

} // namespace crypto
