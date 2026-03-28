/**
 * deck_c_api.h - Pure C interface for sekai-deck-recommend-cpp
 *
 * This is the C shim layer between CGo (Go) and the C++ engine.
 * All cross-language data is passed as UTF-8 JSON strings and raw byte buffers
 * to avoid any ABI / struct-alignment issues.
 *
 * Build: compile alongside sekai_deck_recommend.cpp with the C symbols exported.
 * See: pkg/deck_cgo/BUILD.md for platform-specific build instructions.
 */

#ifndef DECK_C_API_H
#define DECK_C_API_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

#ifdef _WIN32
  #define DECK_API __declspec(dllexport)
#else
  #define DECK_API __attribute__((visibility("default")))
#endif

/** Opaque handle to the SekaiDeckRecommend C++ object */
typedef void* DeckEngineHandle;

/**
 * Create a new deck recommendation engine instance.
 * @return handle or NULL on failure
 */
DECK_API DeckEngineHandle deck_engine_create(void);

/**
 * Destroy a deck engine instance and free all resources.
 * @param handle Handle returned by deck_engine_create
 */
DECK_API void deck_engine_destroy(DeckEngineHandle handle);

/**
 * Set the static data directory used by the engine for bundled extra data files
 * such as worldBloomSupportDeckBonusesWL1/WL2.
 * @param data_dir  Directory path containing static JSON files (NUL-terminated)
 * @return 0 on success, non-zero on error
 */
DECK_API int deck_set_static_data_dir(const char* data_dir);

/**
 * Update master data from a local directory.
 * @param handle  Engine handle
 * @param base_dir  Base directory of masterdata JSON files (NUL-terminated)
 * @param region  Region string, e.g. "jp", "cn" (NUL-terminated)
 * @return 0 on success, non-zero on error
 */
DECK_API int deck_engine_update_masterdata(
    DeckEngineHandle handle,
    const char* base_dir,
    const char* region
);

/**
 * Update master data from in-memory JSON strings.
 *
 * @param handle    Engine handle
 * @param keys      Array of NUL-terminated key strings (e.g. "cards", "events")
 * @param values    Array of UTF-8 JSON byte buffers
 * @param lengths   Array of byte lengths for each value
 * @param count     Number of key/value pairs
 * @param region    Region string (NUL-terminated)
 * @return 0 on success, non-zero on error
 */
DECK_API int deck_engine_update_masterdata_from_strings(
    DeckEngineHandle handle,
    const char** keys,
    const uint8_t** values,
    const size_t* lengths,
    int count,
    const char* region
);

/**
 * Update music metadata from a local file.
 * @param handle     Engine handle
 * @param file_path  Path to music_metas.json (NUL-terminated)
 * @param region     Region string (NUL-terminated)
 * @return 0 on success, non-zero on error
 */
DECK_API int deck_engine_update_musicmetas(
    DeckEngineHandle handle,
    const char* file_path,
    const char* region
);

/**
 * Update music metadata from an in-memory JSON buffer.
 * @param handle  Engine handle
 * @param data    UTF-8 JSON bytes
 * @param length  Byte length of data
 * @param region  Region string (NUL-terminated)
 * @return 0 on success, non-zero on error
 */
DECK_API int deck_engine_update_musicmetas_from_string(
    DeckEngineHandle handle,
    const uint8_t* data,
    size_t length,
    const char* region
);

/**
 * Run deck recommendation.
 *
 * @param handle       Engine handle
 * @param options_json  UTF-8 JSON string encoding DeckRecommendOptions
 *                      (same schema as DeckRecommendOptions.to_dict())
 * @param options_len  Byte length of options_json
 * @param user_json    UTF-8 JSON bytes of user.json (suite dump), or NULL
 * @param user_len     Byte length of user_json, or 0
 * @return  Heap-allocated UTF-8 JSON result string (DeckRecommendResult.to_dict()).
 *          Caller MUST free with deck_free_string().
 *          Returns NULL on error; use deck_last_error() for details.
 */
DECK_API char* deck_engine_recommend(
    DeckEngineHandle handle,
    const uint8_t* options_json,
    size_t options_len,
    const uint8_t* user_json,
    size_t user_len
);

/**
 * Retrieve the last error message, if any.
 * @param handle  Engine handle
 * @return NUL-terminated error string (valid until next call). Do NOT free.
 */
DECK_API const char* deck_last_error(DeckEngineHandle handle);

/**
 * Free a string returned by deck_engine_recommend().
 * @param str Pointer returned by deck_engine_recommend
 */
DECK_API void deck_free_string(char* str);

#ifdef __cplusplus
}
#endif

#endif /* DECK_C_API_H */
