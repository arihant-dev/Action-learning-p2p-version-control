#include "sha256.h"
#include <fstream>
#include <sstream>
#include <iomanip>
#include <cstdint>
#include <cstring>

namespace crypto {

class SHA256 {
private:
    uint32_t state[8];
    uint64_t bit_count;
    uint8_t buffer[64];

    static inline uint32_t rotr(uint32_t x, uint32_t n) {
        return (x >> n) | (x << (32 - n));
    }

    static inline uint32_t choose(uint32_t x, uint32_t y, uint32_t z) {
        return (x & y) ^ (~x & z);
    }

    static inline uint32_t majority(uint32_t x, uint32_t y, uint32_t z) {
        return (x & y) ^ (x & z) ^ (y & z);
    }

    static inline uint32_t sig0(uint32_t x) { return rotr(x, 7) ^ rotr(x, 18) ^ (x >> 3); }
    static inline uint32_t sig1(uint32_t x) { return rotr(x, 17) ^ rotr(x, 19) ^ (x >> 10); }
    static inline uint32_t sum0(uint32_t x) { return rotr(x, 2) ^ rotr(x, 13) ^ rotr(x, 22); }
    static inline uint32_t sum1(uint32_t x) { return rotr(x, 6) ^ rotr(x, 11) ^ rotr(x, 25); }

    void transform(const uint8_t *block) {
        uint32_t w[64];
        for (int i = 0; i < 16; ++i) {
            w[i] = (block[i * 4] << 24) | (block[i * 4 + 1] << 16) | (block[i * 4 + 2] << 8) | block[i * 4 + 3];
        }
        for (int i = 16; i < 64; ++i) {
            w[i] = sig1(w[i - 2]) + w[i - 7] + sig0(w[i - 15]) + w[i - 16];
        }

        uint32_t a = state[0], b = state[1], c = state[2], d = state[3];
        uint32_t e = state[4], f = state[5], g = state[6], h = state[7];

        static const uint32_t k[64] = {
            0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
            0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
            0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
            0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
            0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
            0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
            0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
            0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2
        };

        for (int i = 0; i < 64; ++i) {
            uint32_t temp1 = h + sum1(e) + choose(e, f, g) + k[i] + w[i];
            uint32_t temp2 = sum0(a) + majority(a, b, c);
            h = g; g = f; f = e;
            e = d + temp1;
            d = c; c = b; b = a;
            a = temp1 + temp2;
        }

        state[0] += a; state[1] += b; state[2] += c; state[3] += d;
        state[4] += e; state[5] += f; state[6] += g; state[7] += h;
    }

public:
    SHA256() {
        state[0] = 0x6a09e667; state[1] = 0xbb67ae85; state[2] = 0x3c6ef372; state[3] = 0xa54ff53a;
        state[4] = 0x510e527f; state[5] = 0x9b05688c; state[6] = 0x1f83d9ab; state[7] = 0x5be0cd19;
        bit_count = 0;
    }

    void update(const uint8_t *data, size_t len) {
        size_t offset = 0;
        // Handle partial block from previous call
        size_t buf_off = (bit_count / 8) % 64;
        if (buf_off > 0) {
            size_t to_copy = std::min(len, 64 - buf_off);
            std::memcpy(buffer + buf_off, data, to_copy);
            bit_count += to_copy * 8;
            offset += to_copy;
            if (buf_off + to_copy == 64) {
                transform(buffer);
            } else {
                return; // Still not enough for a full block
            }
        }
        // Process full 64-byte blocks in bulk
        while (offset + 64 <= len) {
            std::memcpy(buffer, data + offset, 64);
            bit_count += 512;
            transform(buffer);
            offset += 64;
        }
        // Store remaining bytes for next call
        if (offset < len) {
            std::memcpy(buffer, data + offset, len - offset);
            bit_count += (len - offset) * 8;
        }
    }

    void finalize(uint8_t digest[32]) {
        uint64_t total_bits = bit_count;
        update((const uint8_t *)"\x80", 1);
        while (bit_count % 512 != 448) {
            update((const uint8_t *)"\x00", 1);
        }
        uint8_t length_bytes[8];
        for (int i = 0; i < 8; ++i) {
            length_bytes[i] = (total_bits >> (56 - i * 8)) & 0xFF;
        }
        update(length_bytes, 8);

        for (int i = 0; i < 8; ++i) {
            digest[i * 4] = (state[i] >> 24) & 0xFF;
            digest[i * 4 + 1] = (state[i] >> 16) & 0xFF;
            digest[i * 4 + 2] = (state[i] >> 8) & 0xFF;
            digest[i * 4 + 3] = state[i] & 0xFF;
        }
    }
};

std::string sha256(const std::string &input) {
    SHA256 hasher;
    hasher.update((const uint8_t *)input.data(), input.size());
    uint8_t digest[32];
    hasher.finalize(digest);

    std::stringstream ss;
    for (int i = 0; i < 32; ++i) {
        ss << std::hex << std::setw(2) << std::setfill('0') << (int)digest[i];
    }
    return ss.str();
}

std::string sha256_file(const std::string &path) {
    std::ifstream file(path, std::ios::binary);
    if (!file) {
        return "";
    }

    SHA256 hasher;
    char buffer[4096];
    while (file.read(buffer, sizeof(buffer))) {
        hasher.update((const uint8_t *)buffer, file.gcount());
    }
    hasher.update((const uint8_t *)buffer, file.gcount());

    uint8_t digest[32];
    hasher.finalize(digest);

    std::stringstream ss;
    for (int i = 0; i < 32; ++i) {
        ss << std::hex << std::setw(2) << std::setfill('0') << (int)digest[i];
    }
    return ss.str();
}

} // namespace crypto
