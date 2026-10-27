//go:build ios && cgo
#include "native.h"
#include "_cgo_export.h"
#include <string.h>
extern void mygo_swift_browser_open(uint64_t,const char*) __attribute__((weak_import));
extern void mygo_swift_browser_cancel(uint64_t) __attribute__((weak_import));
extern void mygo_swift_activity(uint64_t,const char*,const char*) __attribute__((weak_import));
extern void mygo_swift_activity_cancel(uint64_t) __attribute__((weak_import));
void mygo_ios_experience(uint64_t token,const char *kind,const char *json){
 if(strcmp(kind,"browser")==0 && mygo_swift_browser_open){mygo_swift_browser_open(token,json);return;}
 if(strcmp(kind,"browser")!=0 && mygo_swift_activity){mygo_swift_activity(token,kind,json);return;}
 goIOSSystemResult(token,"null",1,"");
}
void mygo_ios_experience_cancel(uint64_t token,const char *kind){
 if(strcmp(kind,"browser")==0 && mygo_swift_browser_cancel){mygo_swift_browser_cancel(token);return;}
 if(mygo_swift_activity_cancel)mygo_swift_activity_cancel(token);
}
void mygo_ios_live_activity_event(const char *json){goIOSLiveActivityEvent((char*)json);}
