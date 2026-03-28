cmake_minimum_required(VERSION 3.15)
project(sekai_deck_recommend_c VERSION 1.0.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# ── Find sekai-deck-recommend-cpp upstream ─────────────────────────────────
# Expected layout after cloning with --recursive:
#
#   vendor/sekai-deck-recommend-cpp/
#     src/sekai_deck_recommend.cpp
#     src/sekai_deck_recommend.hpp   (may need to be extracted from .cpp)
#     3rdparty/json/single_include/
#     3rdparty/pybind11/             (NOT used here)
#
set(UPSTREAM_DIR "${CMAKE_CURRENT_SOURCE_DIR}/vendor/sekai-deck-recommend-cpp"
    CACHE PATH "Path to the sekai-deck-recommend-cpp source tree")

if(NOT EXISTS "${UPSTREAM_DIR}/src/sekai_deck_recommend.cpp")
    message(FATAL_ERROR
        "sekai-deck-recommend-cpp source not found at ${UPSTREAM_DIR}.\n"
        "Clone it with:\n"
        "  git clone --recursive https://github.com/NeuraXmy/sekai-deck-recommend-cpp "
        "  ${UPSTREAM_DIR}")
endif()

# ── Include paths ───────────────────────────────────────────────────────────
include_directories(
    ${CMAKE_CURRENT_SOURCE_DIR}                         # for deck_c_api.h
    ${UPSTREAM_DIR}/src
    ${UPSTREAM_DIR}/3rdparty/json/single_include
)

# ── Source files ────────────────────────────────────────────────────────────
# Collect all upstream .cpp files EXCEPT the python binding entry-point.
file(GLOB_RECURSE UPSTREAM_SRCS "${UPSTREAM_DIR}/src/*.cpp")
list(FILTER UPSTREAM_SRCS EXCLUDE REGEX "sekai_deck_recommend\\.cpp$")

set(SHIM_SRC "${CMAKE_CURRENT_SOURCE_DIR}/csrc/deck_c_api.cpp")

# ── Shared library ──────────────────────────────────────────────────────────
add_library(sekai_deck_recommend_c SHARED
    ${UPSTREAM_SRCS}
    ${SHIM_SRC}
)

# Export only deck_c_api symbols; hide C++ internals.
set_target_properties(sekai_deck_recommend_c PROPERTIES
    C_VISIBILITY_PRESET   hidden
    CXX_VISIBILITY_PRESET hidden
    VISIBILITY_INLINES_HIDDEN ON
)

# ── Compiler flags ──────────────────────────────────────────────────────────
if(MSVC)
    target_compile_options(sekai_deck_recommend_c PRIVATE
        /W3 /MP /O2
        /D_CRT_SECURE_NO_WARNINGS
    )
else()
    target_compile_options(sekai_deck_recommend_c PRIVATE
        -Wall -Wextra -O3 -fPIC
    )
endif()

# ── Install ─────────────────────────────────────────────────────────────────
# Outputs go into internal/pjsk/render/deck/deck_cgo/lib/<platform>/
# so Go CGo can find them.
if(WIN32)
    set(PLATFORM "windows_amd64")
elseif(APPLE)
    if(CMAKE_SYSTEM_PROCESSOR MATCHES "arm64|aarch64")
        set(PLATFORM "darwin_arm64")
    else()
        set(PLATFORM "darwin_amd64")
    endif()
else()
    if(CMAKE_SYSTEM_PROCESSOR MATCHES "arm64|aarch64")
        set(PLATFORM "linux_arm64")
    else()
        set(PLATFORM "linux_amd64")
    endif()
endif()

install(TARGETS sekai_deck_recommend_c
    RUNTIME DESTINATION "${CMAKE_CURRENT_SOURCE_DIR}/lib/${PLATFORM}"  # .dll
    LIBRARY DESTINATION "${CMAKE_CURRENT_SOURCE_DIR}/lib/${PLATFORM}"  # .so
    ARCHIVE DESTINATION "${CMAKE_CURRENT_SOURCE_DIR}/lib/${PLATFORM}"  # .lib
)

message(STATUS "Platform: ${PLATFORM}")
message(STATUS "Output:   internal/pjsk/render/deck/deck_cgo/lib/${PLATFORM}/")
