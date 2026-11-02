//go:build ios && cgo
#include "native.h"
#include "_cgo_export.h"
#include <string.h>
// The Go c-archive path does not preserve undefined weak imports reliably.
// Concrete weak fallbacks also let a host opt into one helper independently;
// a selected Swift source supplies the strong implementation at final link.
__attribute__((weak)) void mygo_swift_browser_open(uint64_t token,const char *json){goIOSSystemResult(token,"null",1,"");}
__attribute__((weak)) void mygo_swift_browser_cancel(uint64_t token){goIOSSystemResult(token,"null",1,"");}
__attribute__((weak)) void mygo_swift_activity(uint64_t token,const char *kind,const char *json){goIOSSystemResult(token,"null",1,"");}
__attribute__((weak)) void mygo_swift_activity_cancel(uint64_t token){goIOSSystemResult(token,"null",1,"");}
void mygo_ios_experience(uint64_t token,const char *kind,const char *json){
 if(strcmp(kind,"browser")==0){mygo_swift_browser_open(token,json);return;}
 if(strcmp(kind,"browser")!=0){mygo_swift_activity(token,kind,json);return;}
 goIOSSystemResult(token,"null",1,"");
}
void mygo_ios_experience_cancel(uint64_t token,const char *kind){
 if(strcmp(kind,"browser")==0){mygo_swift_browser_cancel(token);return;}
 mygo_swift_activity_cancel(token);
}
void mygo_ios_live_activity_event(const char *json){goIOSLiveActivityEvent((char*)json);}
