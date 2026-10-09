//go:build ios && cgo

#import "native.h"
#import "_cgo_export.h"
#import <Foundation/Foundation.h>
#if MYGO_IOS_BIOMETRICS
#import <LocalAuthentication/LocalAuthentication.h>

// UIKit-thread registry. Removing before completion makes explicit cancel,
// background cancellation and LocalAuthentication's late reply idempotent.
static LAContext *authenticationContext;
static uint64_t authenticationToken;

static NSString *authenticationCode(NSError *error) {
  if (![error.domain isEqual:LAErrorDomain]) return @"unknown";
  switch (error.code) {
    case LAErrorUserCancel: case LAErrorSystemCancel: case LAErrorAppCancel: return @"canceled";
    case LAErrorAuthenticationFailed: return @"failed";
    case LAErrorBiometryNotAvailable: return @"unavailable";
    case LAErrorBiometryNotEnrolled: return @"not-enrolled";
    case LAErrorBiometryLockout: return @"locked-out";
    case LAErrorPasscodeNotSet: return @"passcode-not-set";
    case LAErrorUserFallback: return @"fallback";
    case LAErrorNotInteractive: return @"inactive";
    default: return @"unknown";
  }
}
static void authenticationResult(uint64_t token, NSDictionary *result) {
  NSData *data = [NSJSONSerialization dataWithJSONObject:result options:0 error:nil];
  NSString *json = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
  goIOSSystemResult(token, (char *)json.UTF8String, 0, NULL);
}
static void authenticationFailure(uint64_t token, NSString *code, NSString *message) {
  authenticationResult(token, @{ @"Code": code, @"Message": message ?: @"" });
}
static void finishAuthentication(uint64_t token, BOOL success, NSString *code, NSString *message) {
  if (token != authenticationToken || !authenticationContext) return;
  LAContext *context = authenticationContext;
  authenticationContext = nil;
  authenticationToken = 0;
  [context invalidate];
  if (success) authenticationResult(token, @{});
  else authenticationFailure(token, code, message);
}
bool mygo_ios_auth_pending(void) { return authenticationContext != nil; }
void mygo_ios_auth_cancel(uint64_t token) {
  finishAuthentication(token, NO, @"canceled", @"Authentication canceled.");
}
void mygo_ios_auth_cancel_all(void) { mygo_ios_auth_cancel(authenticationToken); }

void mygo_ios_auth_query(uint64_t token) {
  dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
    @autoreleasepool {
      LAContext *context = [LAContext new];
      NSError *error = nil;
      BOOL available = [context canEvaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics error:&error];
      NSString *kind = context.biometryType == LABiometryTypeFaceID ? @"face-id" : context.biometryType == LABiometryTypeTouchID ? @"touch-id" : @"none";
      BOOL device = [context canEvaluatePolicy:LAPolicyDeviceOwnerAuthentication error:nil];
      NSDictionary *result = @{ @"Kind": kind, @"Available": @(available),
        @"UnavailableReason": available ? @"" : authenticationCode(error),
        @"DeviceAuthenticationAvailable": @(device) };
      [context invalidate];
      dispatch_async(dispatch_get_main_queue(), ^{ authenticationResult(token, result); });
    }
  });
}
void mygo_ios_authenticate(uint64_t token, const char *json) {
  if (mygo_ios_auth_pending() || mygo_ios_auth_state() == 2) {
    authenticationFailure(token, @"busy", @"Another system presentation is active."); return;
  }
  if (mygo_ios_auth_state() != 0) {
    authenticationFailure(token, @"inactive", @"Authentication requires an active Scene."); return;
  }
  NSDictionary *options = [NSJSONSerialization JSONObjectWithData:[[NSString stringWithUTF8String:json] dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];
  LAContext *context = [LAContext new];
  BOOL passcode = [options[@"AllowDevicePasscode"] boolValue];
  if (!passcode) context.localizedFallbackTitle = @"";
  LAPolicy policy = passcode ? LAPolicyDeviceOwnerAuthentication : LAPolicyDeviceOwnerAuthenticationWithBiometrics;
  authenticationContext = context;
  authenticationToken = token;
  // Preflight can wait for securityd. Keep it off UIKit's thread.
  dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
    NSError *error = nil;
    BOOL available = [context canEvaluatePolicy:policy error:&error];
    LABiometryType kind = context.biometryType;
    dispatch_async(dispatch_get_main_queue(), ^{
      if (authenticationToken != token || authenticationContext != context) return;
      if (!available) {
        finishAuthentication(token, NO, authenticationCode(error), error.localizedDescription); return;
      }
      id purpose = [NSBundle.mainBundle objectForInfoDictionaryKey:@"NSFaceIDUsageDescription"];
      BOOL validPurpose = [purpose isKindOfClass:NSString.class] && [[purpose stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet] length] > 0;
      if (kind == LABiometryTypeFaceID && !validPurpose) {
        finishAuthentication(token, NO, @"missing-purpose", @"Face ID requires NSFaceIDUsageDescription in ios.infoPlist."); return;
      }
      if (mygo_ios_auth_state() != 0) {
        finishAuthentication(token, NO, @"inactive", @"Authentication requires an active Scene."); return;
      }
      [context evaluatePolicy:policy localizedReason:options[@"Reason"] reply:^(BOOL success, NSError *replyError) {
        // LocalAuthentication replies on its own queue, never directly into Go.
        dispatch_async(dispatch_get_main_queue(), ^{
          finishAuthentication(token, success, authenticationCode(replyError), replyError.localizedDescription);
        });
      }];
    });
  });
}
#else
bool mygo_ios_auth_pending(void) { return false; }
void mygo_ios_auth_cancel(uint64_t token) {}
void mygo_ios_auth_cancel_all(void) {}
void mygo_ios_auth_query(uint64_t token) { goIOSSystemResult(token, NULL, 1, NULL); }
void mygo_ios_authenticate(uint64_t token, const char *json) { goIOSSystemResult(token, NULL, 1, NULL); }
#endif
