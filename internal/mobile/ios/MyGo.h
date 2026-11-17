// The C interface between MyGo's Go code (internal/mobile) and the Swift
// host. Messages are encoded as package wire encodes them.
#ifndef MYGO_H
#define MYGO_H

#include <stddef.h>
#include <stdint.h>

typedef struct {
	// call runs an encoded call on the main thread and sets *reply to its
	// encoded reply, malloc'd (Go frees it), or NULL for none.
	void (*call)(const uint8_t *msg, size_t len, uint8_t **reply, size_t *reply_len);
	// post asks the main thread to call mygo_drain soon. Any thread.
	void (*post)(void);
	// log writes a line of the app's output to the system's log. Any thread.
	void (*log)(int error, const uint8_t *line, size_t len);
} mygo_host;

// mygo_start starts Go's main function on a thread of its own, once the
// app has launched. Main thread, once.
void mygo_start(mygo_host *host);
// mygo_event sends an encoded event. For an event Go answers, it waits for
// the answer and sets *reply to it, malloc'd (free it with mygo_free);
// on_main tells that the caller is the main thread, which runs Go's calls
// while it waits.
void mygo_event(uint8_t *msg, size_t len, int on_main, uint8_t **reply, size_t *reply_len);
// mygo_drain runs the calls Go made. Main thread.
void mygo_drain(void);
void mygo_free(uint8_t *p);

#endif
