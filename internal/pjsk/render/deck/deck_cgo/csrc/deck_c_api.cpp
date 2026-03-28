/**
 * deck_c_api.cpp
 *
 * Implementation of the pure-C shim calling SekaiDeckRecommend C++ objects.
 * Compile this file together with sekai_deck_recommend.cpp (from the upstream
 * repo) to produce a standalone shared library: sekai_deck_recommend_c.dll /
 * libsekai_deck_recommend_c.so
 *
 * The Python / pybind11 binding (sekai_deck_recommend.cpp pybind11 module) is
 * NOT needed when building this shim; just include the core C++ headers.
 *
 * Upstream: https://github.com/NeuraXmy/sekai-deck-recommend-cpp
 */

#include "deck_c_api.h"

// Pull in the upstream C++ engine headers.
// Adjust the include path to match where you cloned sekai-deck-recommend-cpp.
// When CMake is used, these are included via target_include_directories().
#include "sekai_deck_recommend_pure.hpp"

#include <string>
#include <vector>
#include <stdexcept>
#include <cstring>
#include <cstdlib>

// ─── Internal state per engine instance ──────────────────────────────────────

struct DeckEngineState {
    SekaiDeckRecommend engine;
    std::string last_error;
};

static DeckEngineState* cast(DeckEngineHandle h) {
    return static_cast<DeckEngineState*>(h);
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

static char* dup_string(const std::string& s) {
    char* buf = static_cast<char*>(std::malloc(s.size() + 1));
    if (buf) std::memcpy(buf, s.c_str(), s.size() + 1);
    return buf;
}

// ─── C API implementation ─────────────────────────────────────────────────────

extern "C" {

DECK_API DeckEngineHandle deck_engine_create(void) {
    try {
        return new DeckEngineState();
    } catch (...) {
        return nullptr;
    }
}

DECK_API void deck_engine_destroy(DeckEngineHandle handle) {
    delete cast(handle);
}

DECK_API int deck_set_static_data_dir(const char* data_dir) {
    try {
        if (data_dir == nullptr) {
            return -1;
        }
        init_data_path(std::string(data_dir));
        return 0;
    } catch (...) {
        return -1;
    }
}

DECK_API int deck_engine_update_masterdata(
    DeckEngineHandle handle,
    const char* base_dir,
    const char* region
) {
    auto* s = cast(handle);
    try {
        s->engine.update_masterdata(std::string(base_dir), std::string(region));
        return 0;
    } catch (const std::exception& e) {
        s->last_error = e.what();
        return -1;
    } catch (...) {
        s->last_error = "unknown error in update_masterdata";
        return -1;
    }
}

DECK_API int deck_engine_update_masterdata_from_strings(
    DeckEngineHandle handle,
    const char** keys,
    const uint8_t** values,
    const size_t* lengths,
    int count,
    const char* region
) {
    auto* s = cast(handle);
    try {
        std::unordered_map<std::string, std::string> data;
        for (int i = 0; i < count; ++i) {
            data[std::string(keys[i])] = std::string(
                reinterpret_cast<const char*>(values[i]), lengths[i]
            );
        }
        s->engine.update_masterdata_from_strings(data, std::string(region));
        return 0;
    } catch (const std::exception& e) {
        s->last_error = e.what();
        return -1;
    } catch (...) {
        s->last_error = "unknown error in update_masterdata_from_strings";
        return -1;
    }
}

DECK_API int deck_engine_update_musicmetas(
    DeckEngineHandle handle,
    const char* file_path,
    const char* region
) {
    auto* s = cast(handle);
    try {
        s->engine.update_musicmetas(std::string(file_path), std::string(region));
        return 0;
    } catch (const std::exception& e) {
        s->last_error = e.what();
        return -1;
    } catch (...) {
        s->last_error = "unknown error in update_musicmetas";
        return -1;
    }
}

DECK_API int deck_engine_update_musicmetas_from_string(
    DeckEngineHandle handle,
    const uint8_t* data,
    size_t length,
    const char* region
) {
    auto* s = cast(handle);
    try {
        std::string json_str(reinterpret_cast<const char*>(data), length);
        s->engine.update_musicmetas_from_string(json_str, std::string(region));
        return 0;
    } catch (const std::exception& e) {
        s->last_error = e.what();
        return -1;
    } catch (...) {
        s->last_error = "unknown error in update_musicmetas_from_string";
        return -1;
    }
}

DECK_API char* deck_engine_recommend(
    DeckEngineHandle handle,
    const uint8_t* options_json,
    size_t options_len,
    const uint8_t* user_json,
    size_t user_len
) {
    auto* s = cast(handle);
    try {
        std::string opts_str(reinterpret_cast<const char*>(options_json), options_len);

        // Build DeckRecommendOptions from JSON dict
        PyDeckRecommendOptions options = PyDeckRecommendOptions::from_dict(
            nlohmann::json::parse(opts_str)
        );

        // Inject user data directly from bytes if provided
        if (user_json && user_len > 0) {
            options.user_data_str = std::string(
                reinterpret_cast<const char*>(user_json), user_len
            );
        }

        PyDeckRecommendResult result = s->engine.recommend(options);

        // Serialize result to JSON and return as heap-allocated C string
        nlohmann::json result_json = result.to_dict();
        return dup_string(result_json.dump());

    } catch (const std::exception& e) {
        s->last_error = e.what();
        return nullptr;
    } catch (...) {
        s->last_error = "unknown error in recommend";
        return nullptr;
    }
}

DECK_API const char* deck_last_error(DeckEngineHandle handle) {
    return cast(handle)->last_error.c_str();
}

DECK_API void deck_free_string(char* str) {
    std::free(str);
}

} // extern "C"
