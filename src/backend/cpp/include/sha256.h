#ifndef SHA256_H
#define SHA256_H

#include <string>

namespace crypto {

std::string sha256(const std::string& input);
bool sha256_file(const std::string& path, std::string& hash_out);

} // namespace crypto

#endif
