#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
// Protected APIs are opt-in, including for direct Go builds without the CLI.
#ifndef MYGO_IOS_CAMERA
#define MYGO_IOS_CAMERA 0
#endif
#ifndef MYGO_IOS_MICROPHONE
#define MYGO_IOS_MICROPHONE 0
#endif
#ifndef MYGO_IOS_GEOLOCATION
#define MYGO_IOS_GEOLOCATION 0
#endif
#ifndef MYGO_IOS_PHOTOS
#define MYGO_IOS_PHOTOS 0
#endif
#ifndef MYGO_IOS_BIOMETRICS
#define MYGO_IOS_BIOMETRICS 0
#endif
int mygo_ios_start(void);
void mygo_ios_ready(void);
void mygo_ios_attach(uintptr_t view);
void mygo_ios_watch_drawable(uintptr_t view, uintptr_t drawable);
bool mygo_ios_is_main(void);
void mygo_ios_dispatch(uint64_t token);
void mygo_ios_step(void);
void mygo_ios_wake(void);
uintptr_t mygo_ios_create(uint64_t id);
void mygo_ios_close(uintptr_t view);
void mygo_ios_show(uintptr_t view, bool visible);
void mygo_ios_background(uintptr_t view, double r, double g, double b, double a);
uintptr_t mygo_ios_layer(uintptr_t view);
void mygo_ios_size(uintptr_t view, double *w, double *h, double *scale);
double mygo_ios_hz(uintptr_t view);
void mygo_ios_frame(uintptr_t view);
void mygo_ios_pixels(uintptr_t view, const void *pixels, int stride, int w,
                     int h);
void mygo_ios_input(uintptr_t view, bool active, bool readonly, bool password, bool multiline,
                    const char *text, int start, int end, double x, double y,
                    double w, double h);
void mygo_ios_input_options(uintptr_t view, const char *json);
void mygo_ios_input_bounds(uintptr_t view, const char *id, double x, double y, double w, double h);
bool mygo_ios_native_selection(void);
void mygo_ios_input_history(uintptr_t view, bool undo, bool redo);
void mygo_ios_input_accessory(uintptr_t view, uint64_t owner, const char *json);
void mygo_ios_preferences(bool *motion, bool *contrast, double *scale);
void mygo_ios_access(uintptr_t view, const char *json);
bool mygo_ios_dark(void);
void mygo_ios_theme(int mode);
char *mygo_ios_clipboard(void);
void mygo_ios_set_clipboard(const char *text);
uint32_t mygo_ios_clipboard_formats(void);
int64_t mygo_ios_clipboard_change(void);
void *mygo_ios_clipboard_png(size_t *length);
bool mygo_ios_set_clipboard_data(const char *text, const void *png, size_t length);
char *mygo_ios_locale(void);
char *mygo_ios_home(void);
bool mygo_ios_open(const char *url);
void mygo_ios_memory(uintptr_t view);
void mygo_ios_message(uint64_t token, const char *json);
void mygo_ios_share(uint64_t token, const char *json);
void mygo_ios_permission(uint64_t token, const char *kind, bool request);
void mygo_ios_settings(uint64_t token);
void mygo_ios_secret(uint64_t token, const char *json);
void mygo_ios_auth_query(uint64_t token);
void mygo_ios_authenticate(uint64_t token, const char *json);
void mygo_ios_auth_cancel(uint64_t token);
void mygo_ios_auth_cancel_all(void);
bool mygo_ios_auth_pending(void);
int mygo_ios_auth_state(void);

void mygo_ios_open_files(uint64_t token, const char *options);
void mygo_ios_export_files(uint64_t token, const char *options);
void mygo_ios_photos(uint64_t token, const char *options);

void mygo_ios_notification(uint64_t token, const char *json);
void mygo_ios_remove_notification(const char *id);
void mygo_ios_clear_notifications(void);
void mygo_ios_notifications_init(void);
void mygo_ios_notifications_ready(void);
void mygo_ios_notification_response(uintptr_t response);
void mygo_ios_badge(uint64_t token, int count);
void mygo_ios_register_push(void);
void mygo_ios_haptic(const char *kind);
void mygo_ios_status_bar(const char *style, bool hidden);

int mygo_ios_badge_count(void);
bool mygo_ios_can_request(uint64_t token, const char *key);
void mygo_ios_dismiss_keyboard(void);
char *mygo_ios_device(void);
void mygo_ios_scan(uint64_t token, const char *json);
void mygo_ios_scan_cancel(uint64_t token);
void mygo_ios_network(uint64_t token, const char *url, double timeout);
void mygo_ios_network_cancel(uint64_t token);
