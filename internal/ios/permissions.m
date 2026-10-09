//go:build ios && cgo

#import "native.h"
#import "_cgo_export.h"
#import <UIKit/UIKit.h>
#import <AVFoundation/AVFoundation.h>
#import <Photos/Photos.h>
#import <CoreLocation/CoreLocation.h>
#import <UserNotifications/UserNotifications.h>

static void permissionResult(uint64_t token, NSString *status, NSError *error) {
  void (^finish)(void) = ^{
    NSData *data = status ? [NSJSONSerialization dataWithJSONObject:status options:NSJSONWritingFragmentsAllowed error:nil] : nil;
    NSString *json = data ? [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] : nil;
    goIOSSystemResult(token, (char *)json.UTF8String, error ? 2 : 0, (char *)error.localizedDescription.UTF8String);
  };
  if (NSThread.isMainThread) finish();
  else dispatch_async(dispatch_get_main_queue(), finish);
}
static NSError *permissionError(NSString *message) {
  return [NSError errorWithDomain:@"MyGoPermissions" code:1 userInfo:@{NSLocalizedDescriptionKey: message}];
}
static BOOL canRequest(uint64_t token, NSString *key) {
  if (key) {
    id purpose = NSBundle.mainBundle.infoDictionary[key];
    if (![purpose isKindOfClass:NSString.class] || ![purpose stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet].length) {
      permissionResult(token, nil, permissionError([NSString stringWithFormat:@"mygo: permission request requires %@ in ios.infoPlist", key]));
      return NO;
    }
  }
  if (UIApplication.sharedApplication.applicationState != UIApplicationStateActive) {
    permissionResult(token, nil, permissionError(@"mygo: permission request requires an active application"));
    return NO;
  }
  return YES;
}
bool mygo_ios_can_request(uint64_t token, const char *key) {
  return canRequest(token, key ? [NSString stringWithUTF8String:key] : nil);
}
static NSString *captureStatus(AVAuthorizationStatus status) {
  switch (status) {
    case AVAuthorizationStatusAuthorized: return @"granted";
    case AVAuthorizationStatusDenied: return @"denied";
    case AVAuthorizationStatusRestricted: return @"restricted";
    default: return @"not-determined";
  }
}
static NSString *photoStatus(PHAuthorizationStatus status) {
  switch (status) {
    case PHAuthorizationStatusAuthorized: return @"granted";
    case PHAuthorizationStatusLimited: return @"limited";
    case PHAuthorizationStatusDenied: return @"denied";
    case PHAuthorizationStatusRestricted: return @"restricted";
    default: return @"not-determined";
  }
}
static NSString *locationStatus(CLAuthorizationStatus status) {
  switch (status) {
    case kCLAuthorizationStatusAuthorizedAlways:
    case kCLAuthorizationStatusAuthorizedWhenInUse: return @"granted";
    case kCLAuthorizationStatusDenied: return @"denied";
    case kCLAuthorizationStatusRestricted: return @"restricted";
    default: return @"not-determined";
  }
}
static NSString *notificationStatus(UNAuthorizationStatus status) {
  switch (status) {
    case UNAuthorizationStatusAuthorized:
    case UNAuthorizationStatusEphemeral: return @"granted";
    case UNAuthorizationStatusProvisional: return @"provisional";
    case UNAuthorizationStatusDenied: return @"denied";
    default: return @"not-determined";
  }
}

@interface MyGoLocationPermission : NSObject <CLLocationManagerDelegate>
@property(nonatomic, strong) CLLocationManager *manager;
@property(nonatomic, strong) NSMutableArray<NSNumber *> *tokens;
@end
static MyGoLocationPermission *locationPermission;
@implementation MyGoLocationPermission
- (void)locationManagerDidChangeAuthorization:(CLLocationManager *)manager {
  if (manager.authorizationStatus == kCLAuthorizationStatusNotDetermined) return;
  NSArray<NSNumber *> *tokens = self.tokens.copy;
  [self.tokens removeAllObjects];
  for (NSNumber *token in tokens) permissionResult(token.unsignedLongLongValue, locationStatus(manager.authorizationStatus), nil);
}
@end

void mygo_ios_permission(uint64_t token, const char *kind, bool request) {
  NSString *name = [NSString stringWithUTF8String:kind];
  if ([name isEqual:@"camera"] || [name isEqual:@"microphone"]) {
    AVMediaType media = [name isEqual:@"camera"] ? AVMediaTypeVideo : AVMediaTypeAudio;
    AVAuthorizationStatus status = [AVCaptureDevice authorizationStatusForMediaType:media];
    if (!request || status != AVAuthorizationStatusNotDetermined) { permissionResult(token, captureStatus(status), nil); return; }
    if (!canRequest(token, [name isEqual:@"camera"] ? @"NSCameraUsageDescription" : @"NSMicrophoneUsageDescription")) return;
    [AVCaptureDevice requestAccessForMediaType:media completionHandler:^(BOOL granted) {
      permissionResult(token, captureStatus([AVCaptureDevice authorizationStatusForMediaType:media]), nil);
    }];
    return;
  }
  if ([name isEqual:@"photos"] || [name isEqual:@"photos-add-only"]) {
    PHAccessLevel level = [name isEqual:@"photos"] ? PHAccessLevelReadWrite : PHAccessLevelAddOnly;
    PHAuthorizationStatus status = [PHPhotoLibrary authorizationStatusForAccessLevel:level];
    if (!request || status != PHAuthorizationStatusNotDetermined) { permissionResult(token, photoStatus(status), nil); return; }
    if (!canRequest(token, level == PHAccessLevelReadWrite ? @"NSPhotoLibraryUsageDescription" : @"NSPhotoLibraryAddUsageDescription")) return;
    [PHPhotoLibrary requestAuthorizationForAccessLevel:level handler:^(PHAuthorizationStatus status) { permissionResult(token, photoStatus(status), nil); }];
    return;
  }
  if ([name isEqual:@"geolocation"]) {
    if (!locationPermission) {
      locationPermission = [MyGoLocationPermission new];
      locationPermission.tokens = [NSMutableArray array];
      locationPermission.manager = [CLLocationManager new];
      locationPermission.manager.delegate = locationPermission;
    }
    CLAuthorizationStatus status = locationPermission.manager.authorizationStatus;
    if (!request || status != kCLAuthorizationStatusNotDetermined) { permissionResult(token, locationStatus(status), nil); return; }
    if (!canRequest(token, @"NSLocationWhenInUseUsageDescription")) return;
    [locationPermission.tokens addObject:@(token)];
    if (locationPermission.tokens.count == 1) [locationPermission.manager requestWhenInUseAuthorization];
    return;
  }
  if ([name isEqual:@"notifications"]) {
    UNUserNotificationCenter *center = UNUserNotificationCenter.currentNotificationCenter;
    [center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
      dispatch_async(dispatch_get_main_queue(), ^{
        if (!request || settings.authorizationStatus != UNAuthorizationStatusNotDetermined) { permissionResult(token, notificationStatus(settings.authorizationStatus), nil); return; }
        if (!canRequest(token, nil)) return;
        [center requestAuthorizationWithOptions:UNAuthorizationOptionAlert | UNAuthorizationOptionSound | UNAuthorizationOptionBadge completionHandler:^(BOOL granted, NSError *error) {
          if (error) { permissionResult(token, nil, error); return; }
          [center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) { permissionResult(token, notificationStatus(settings.authorizationStatus), nil); }];
        }];
      });
    }];
    return;
  }
  goIOSSystemResult(token, NULL, 1, NULL);
}
void mygo_ios_settings(uint64_t token) {
  if (!canRequest(token, nil)) return;
  [UIApplication.sharedApplication openURL:[NSURL URLWithString:UIApplicationOpenSettingsURLString] options:@{} completionHandler:^(BOOL success) {
    permissionResult(token, @"", success ? nil : permissionError(@"mygo: could not open application settings"));
  }];
}
