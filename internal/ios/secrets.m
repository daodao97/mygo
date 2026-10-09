//go:build ios && cgo

#import "native.h"
#import "_cgo_export.h"
#import <Foundation/Foundation.h>
#import <Security/Security.h>
#if MYGO_IOS_BIOMETRICS
#import <LocalAuthentication/LocalAuthentication.h>
#endif

void mygo_ios_secret(uint64_t token, const char *json) {
  NSDictionary *o = [NSJSONSerialization JSONObjectWithData:[[NSString stringWithUTF8String:json] dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];
  // Security services may wait for securityd or protected data. Keep those
  // synchronous calls off UIKit's thread, then deliver the result on it.
  dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
    @autoreleasepool {
      NSString *service = [NSString stringWithFormat:@"%@.mygo.%@", NSBundle.mainBundle.bundleIdentifier, o[@"Namespace"]];
      NSMutableDictionary *query = [@{(__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
        (__bridge id)kSecAttrService: service, (__bridge id)kSecAttrAccount: o[@"Key"],
        (__bridge id)kSecAttrSynchronizable: @NO} mutableCopy];
      OSStatus status = errSecParam;
      NSData *value = nil;
      if ([o[@"Operation"] isEqual:@"set"]) {
        NSData *data = [[NSData alloc] initWithBase64EncodedString:o[@"Value"] options:0];
        CFStringRef accessibility = [o[@"Accessibility"] isEqual:@"after-first-unlock"] ? kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly : kSecAttrAccessibleWhenUnlockedThisDeviceOnly;
        NSDictionary *attributes = @{(__bridge id)kSecValueData: data, (__bridge id)kSecAttrAccessible: (__bridge id)accessibility};
        status = SecItemUpdate((__bridge CFDictionaryRef)query, (__bridge CFDictionaryRef)attributes);
        if (status == errSecItemNotFound) {
          NSMutableDictionary *add = query.mutableCopy;
          [add addEntriesFromDictionary:attributes];
          status = SecItemAdd((__bridge CFDictionaryRef)add, NULL);
          // Concurrent writers may both have seen an absent key.
          if (status == errSecDuplicateItem) status = SecItemUpdate((__bridge CFDictionaryRef)query, (__bridge CFDictionaryRef)attributes);
        }
      } else if ([o[@"Operation"] isEqual:@"get"]) {
        query[(__bridge id)kSecReturnData] = @YES;
        query[(__bridge id)kSecMatchLimit] = (__bridge id)kSecMatchLimitOne;
#if MYGO_IOS_BIOMETRICS
        LAContext *context = [LAContext new];
        context.interactionNotAllowed = YES;
        query[(__bridge id)kSecUseAuthenticationContext] = context;
#else
        // Ordinary Keychain access must not link Face ID APIs or show UI.
        query[(__bridge id)kSecUseAuthenticationUI] = (__bridge id)kSecUseAuthenticationUIFail;
#endif
        CFTypeRef result = NULL;
        status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &result);
        if (result) value = CFBridgingRelease(result);
      } else if ([o[@"Operation"] isEqual:@"delete"]) {
        status = SecItemDelete((__bridge CFDictionaryRef)query);
        if (status == errSecItemNotFound) status = errSecSuccess;
      }
      NSString *encoded = value ? [value base64EncodedStringWithOptions:0] : nil;
      NSData *result = [NSJSONSerialization dataWithJSONObject:encoded ?: NSNull.null options:NSJSONWritingFragmentsAllowed error:nil];
      NSString *resultJSON = [[NSString alloc] initWithData:result encoding:NSUTF8StringEncoding];
      NSString *message = status == errSecSuccess ? nil : CFBridgingRelease(SecCopyErrorMessageString(status, NULL));
      dispatch_async(dispatch_get_main_queue(), ^{
        int code = status == errSecSuccess ? 0 : status == errSecItemNotFound ? 3 : 2;
        NSString *detail = [NSString stringWithFormat:@"mygo: keychain error %d: %@", (int)status, message ?: @"unknown error"];
        goIOSSystemResult(token, (char *)resultJSON.UTF8String, code, (char *)detail.UTF8String);
      });
    }
  });
}
