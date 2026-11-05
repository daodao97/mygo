//go:build ios && cgo

#import "native.h"
#import "_cgo_export.h"
#import <Foundation/Foundation.h>
#import <Security/Security.h>
#include <stdlib.h>
#include <string.h>
#if MYGO_IOS_BIOMETRICS
#import <LocalAuthentication/LocalAuthentication.h>
#endif

// keychainQueue runs the keychain calls one at a time, in the order they were
// made. On a concurrent queue two Sets of one key could land in either order,
// and a Get right after a Set could read the value before it.
static dispatch_queue_t keychainQueue(void) {
  static dispatch_queue_t queue;
  static dispatch_once_t once;
  dispatch_once(&once, ^{
    queue = dispatch_queue_create("mygo.keychain", dispatch_queue_attr_make_with_qos_class(DISPATCH_QUEUE_SERIAL, QOS_CLASS_USER_INITIATED, 0));
  });
  return queue;
}

// mygo_ios_keychain runs one Keychain operation, op 1 set, 2 get and 3
// delete, and returns its OSStatus. Secrets calls it off the main thread, as
// Security services may wait for securityd or protected data; it waits on the
// serial queue alone, never on the main thread. A value read is malloc'd into
// out, and the description of a failure into message, for the caller to free.
int mygo_ios_keychain(int op, const char *service, const char *account, const void *bytes, int length, void **out, int *outLength, char **message) {
  *out = NULL;
  *outLength = 0;
  *message = NULL;
  NSString *serviceName = [NSString stringWithUTF8String:service];
  NSString *accountName = [NSString stringWithUTF8String:account];
  NSData *data = op == 1 ? [NSData dataWithBytes:bytes length:(NSUInteger)length] : nil;
  __block OSStatus status = errSecParam;
  __block NSData *value = nil;
  dispatch_sync(keychainQueue(), ^{
    @autoreleasepool {
      NSMutableDictionary *query = [@{(__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
        (__bridge id)kSecAttrService: serviceName, (__bridge id)kSecAttrAccount: accountName,
        (__bridge id)kSecAttrSynchronizable: @NO} mutableCopy];
      if (op == 1) {
        NSDictionary *attributes = @{(__bridge id)kSecValueData: data, (__bridge id)kSecAttrAccessible: (__bridge id)kSecAttrAccessibleWhenUnlockedThisDeviceOnly};
        status = SecItemUpdate((__bridge CFDictionaryRef)query, (__bridge CFDictionaryRef)attributes);
        if (status == errSecItemNotFound) {
          NSMutableDictionary *add = query.mutableCopy;
          [add addEntriesFromDictionary:attributes];
          status = SecItemAdd((__bridge CFDictionaryRef)add, NULL);
          // Concurrent writers may both have seen an absent key.
          if (status == errSecDuplicateItem) status = SecItemUpdate((__bridge CFDictionaryRef)query, (__bridge CFDictionaryRef)attributes);
        }
      } else if (op == 2) {
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
      } else if (op == 3) {
        status = SecItemDelete((__bridge CFDictionaryRef)query);
        if (status == errSecItemNotFound) status = errSecSuccess;
      }
    }
  });
  if (status == errSecSuccess && value.length > 0) {
    *out = malloc(value.length);
    memcpy(*out, value.bytes, value.length);
    *outLength = (int)value.length;
  } else if (status != errSecSuccess && status != errSecItemNotFound) {
    NSString *detail = CFBridgingRelease(SecCopyErrorMessageString(status, NULL));
    *message = strdup((detail ?: @"unknown error").UTF8String);
  }
  return (int)status;
}
