# SQLite on Android: the amalgamation, pinned by its hash, and the JNI the
# module's Kotlin calls, in one library, libsqlitecore.so. Its symbols stay
# hidden but for the JNI, as the Zig build's do.
cmake_minimum_required(VERSION 3.22)
project(sqlitecore C)

set(SQLITE_VERSION 3530400)
set(SQLITE_SHA256 1e71ddf93849c6a6ecf58b827c0692073d2dd7ee40196158068f7b29f422e87d)
set(SQLITE_DIR ${SQLITE_DOWNLOADS}/sqlite-amalgamation-${SQLITE_VERSION})
if(NOT EXISTS ${SQLITE_DIR}/sqlite3.c)
  set(zip ${SQLITE_DOWNLOADS}/sqlite-${SQLITE_VERSION}-${ANDROID_ABI}.zip)
  file(DOWNLOAD https://www.sqlite.org/2026/sqlite-amalgamation-${SQLITE_VERSION}.zip ${zip}
    EXPECTED_HASH SHA256=${SQLITE_SHA256} TLS_VERIFY ON STATUS status)
  list(GET status 0 code)
  if(NOT code EQUAL 0)
    message(FATAL_ERROR "downloading SQLite's amalgamation: ${status}")
  endif()
  # Each ABI configures on its own: the first to finish puts its copy in
  # place, whole.
  file(ARCHIVE_EXTRACT INPUT ${zip} DESTINATION ${SQLITE_DOWNLOADS}/${ANDROID_ABI})
  file(RENAME ${SQLITE_DOWNLOADS}/${ANDROID_ABI}/sqlite-amalgamation-${SQLITE_VERSION} ${SQLITE_DIR} RESULT moved)
  file(REMOVE_RECURSE ${SQLITE_DOWNLOADS}/${ANDROID_ABI} ${zip})
endif()

add_library(sqlitecore SHARED ${SQLITE_DIR}/sqlite3.c sqlitecore.c)
target_include_directories(sqlitecore PRIVATE ${SQLITE_DIR})
set_target_properties(sqlitecore PROPERTIES C_STANDARD 11 C_VISIBILITY_PRESET hidden)
target_compile_definitions(sqlitecore PRIVATE
  SQLITE_THREADSAFE=1
  SQLITE_DQS=0
  SQLITE_DEFAULT_FOREIGN_KEYS=1
  SQLITE_ENABLE_FTS5=1
  SQLITE_ENABLE_RTREE=1
  SQLITE_OMIT_LOAD_EXTENSION=1
  SQLITE_OMIT_DEPRECATED=1
  SQLITE_OMIT_SHARED_CACHE=1)
target_link_libraries(sqlitecore PRIVATE m)
