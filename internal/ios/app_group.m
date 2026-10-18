//go:build ios && cgo

#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>

char *mygo_ios_app_group(const char *identifier, char **message) {
    @autoreleasepool {
        NSString *group = [NSString stringWithUTF8String:identifier];
        NSURL *url = [NSFileManager.defaultManager containerURLForSecurityApplicationGroupIdentifier:group];
        if (!url) { *message = strdup("mygo: App Group is unavailable; check the entitlement and provisioning profile"); return NULL; }
        NSError *error;
        if (![NSFileManager.defaultManager setAttributes:@{NSFileProtectionKey:NSFileProtectionCompleteUntilFirstUserAuthentication} ofItemAtPath:url.path error:&error]) {
            *message = strdup(error.localizedDescription.UTF8String ?: "mygo: cannot set App Group file protection"); return NULL;
        }
        return strdup(url.path.UTF8String);
    }
}
