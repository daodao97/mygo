#pragma once
// Register before UIApplicationMain, on its thread. Callbacks run on a utility
// queue except prepare/cancel (main). run must cooperate with cancellation.
void MyGoRegisterBackgroundRefresh(const char *identifier, void (*prepare)(void), int (*run)(void), void (*cancel)(void), double (*nextDelay)(void));
void MyGoScheduleBackgroundRefresh(const char *identifier);
// Caller frees the returned UTF-8 string. Empty means no recorded error.
char *MyGoBackgroundRefreshLastError(const char *identifier);

int MyGoBackgroundRefreshRegistered(const char *identifier);
