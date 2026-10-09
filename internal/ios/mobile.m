//go:build ios && cgo
#import "native.h"
#import "_cgo_export.h"
#import <UIKit/UIKit.h>
#import <UserNotifications/UserNotifications.h>

static BOOL servicesReady;
static NSMutableArray<NSDictionary *> *notificationEvents;
static NSMutableSet<NSString *> *responses;
static NSString *string(const char *s) { return s ? [NSString stringWithUTF8String:s] : @""; }
static void result(uint64_t token, NSError *error, int code) {
 goIOSSystemResult(token,"null",error ? code : 0,(char *)(error.localizedDescription ?: @"").UTF8String);
}
static uint32_t event(NSDictionary *e) {
 if (!servicesReady) {
  if (!notificationEvents) notificationEvents=[NSMutableArray array];
  [notificationEvents addObject:e];
  if (notificationEvents.count>128) [notificationEvents removeObjectAtIndex:0];
  return 15;
 }
 NSData *data=[NSJSONSerialization dataWithJSONObject:e[@"data"] options:0 error:nil];
 NSString *json=[[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
 return goIOSNotification((char *)[e[@"id"] UTF8String],(char *)json.UTF8String,[e[@"clicked"] boolValue], [e[@"remote"] boolValue], (char *)[e[@"action"] UTF8String]);
}
static uint32_t notificationEvent(UNNotification *n,BOOL clicked,NSString *action) {
 if (clicked) {
  if (!responses) responses=[NSMutableSet set];
  NSString *key=[NSString stringWithFormat:@"%@:%f:%@",n.request.identifier,n.date.timeIntervalSince1970,action];
  if ([responses containsObject:key]) return 0;
  if (responses.count>128) [responses removeAllObjects];
  [responses addObject:key];
 }
 NSMutableDictionary *values=[NSMutableDictionary dictionary];
 NSDictionary *info=n.request.content.userInfo;
 NSDictionary *source=[info[@"mygo"] isKindOfClass:NSDictionary.class] ? info[@"mygo"] : info;
 for (id key in source) if ([key isKindOfClass:NSString.class] && [source[key] isKindOfClass:NSString.class]) values[key]=source[key];
 NSString *identity=[info[@"mygoID"] isKindOfClass:NSString.class] ? info[@"mygoID"] : n.request.identifier;
 if ([action isEqual:UNNotificationDefaultActionIdentifier]) action=@"";
 return event(@{@"id":identity,@"data":values,@"clicked":@(clicked),@"remote":@([n.request.trigger isKindOfClass:UNPushNotificationTrigger.class]),@"action":action ?: @""});
}
@interface MyGoNotifications : NSObject <UNUserNotificationCenterDelegate>
@end
@implementation MyGoNotifications
- (void)userNotificationCenter:(UNUserNotificationCenter *)center willPresentNotification:(UNNotification *)n withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completion {
 dispatch_async(dispatch_get_main_queue(), ^{
  uint32_t mask=notificationEvent(n,NO,@"");
  UNNotificationPresentationOptions options=0;
  if (mask & 1) options |= UNNotificationPresentationOptionBanner;
  if (mask & 2) options |= UNNotificationPresentationOptionList;
  if (mask & 4) options |= UNNotificationPresentationOptionSound;
  if (mask & 8) options |= UNNotificationPresentationOptionBadge;
  completion(options);
 });
}
- (void)userNotificationCenter:(UNUserNotificationCenter *)center didReceiveNotificationResponse:(UNNotificationResponse *)r withCompletionHandler:(void (^)(void))completion {
 dispatch_async(dispatch_get_main_queue(), ^{
  if (![r.actionIdentifier isEqual:UNNotificationDismissActionIdentifier]) notificationEvent(r.notification,YES,r.actionIdentifier);
  completion();
 });
}
@end
static MyGoNotifications *delegate;
void mygo_ios_notifications_init(void) {
 delegate=[MyGoNotifications new];
 UNUserNotificationCenter.currentNotificationCenter.delegate=delegate;
}
void mygo_ios_notifications_ready(void) {
 servicesReady=YES;
 NSArray *events=notificationEvents;notificationEvents=nil;
 for (NSDictionary *e in events) event(e);
}
void mygo_ios_notification_response(uintptr_t response) {
 UNNotificationResponse *r=(__bridge UNNotificationResponse *)(void *)response;
 if (r && ![r.actionIdentifier isEqual:UNNotificationDismissActionIdentifier]) notificationEvent(r.notification,YES,r.actionIdentifier);
}
void mygo_ios_notification(uint64_t token,const char *json) {
 NSDictionary *o=[NSJSONSerialization JSONObjectWithData:[string(json) dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nil];
 UNUserNotificationCenter *center=UNUserNotificationCenter.currentNotificationCenter;
 void (^submit)(void)=^{
  UNMutableNotificationContent *c=[UNMutableNotificationContent new];
  c.title=o[@"Title"] ?: @"";c.subtitle=o[@"Subtitle"] ?: @"";c.body=o[@"Body"] ?: @"";
  c.threadIdentifier=o[@"Group"] ?: @"";
  if (![o[@"Silent"] boolValue]) c.sound=UNNotificationSound.defaultSound;
  if ([o[@"Badge"] isKindOfClass:NSNumber.class]) c.badge=o[@"Badge"];
  c.userInfo=@{@"mygo": [o[@"Data"] isKindOfClass:NSDictionary.class] ? o[@"Data"] : @{}};
  double delay=[o[@"DelaySeconds"] doubleValue];
  UNNotificationTrigger *trigger=delay>0 ? [UNTimeIntervalNotificationTrigger triggerWithTimeInterval:MAX(1,delay) repeats:NO] : nil;
  UNNotificationRequest *r=[UNNotificationRequest requestWithIdentifier:o[@"ID"] content:c trigger:trigger];
  [center addNotificationRequest:r withCompletionHandler:^(NSError *error){ dispatch_async(dispatch_get_main_queue(),^{result(token,error,2);}); }];
 };
 [center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings){
  if (settings.authorizationStatus==UNAuthorizationStatusNotDetermined) {
   [center requestAuthorizationWithOptions:UNAuthorizationOptionAlert | UNAuthorizationOptionSound | UNAuthorizationOptionBadge completionHandler:^(BOOL granted,NSError *error){
    if (granted && !error) submit();else dispatch_async(dispatch_get_main_queue(),^{goIOSSystemResult(token,"null",error?2:4,(char *)(error.localizedDescription ?: @"").UTF8String);});
   }];
  } else if (settings.authorizationStatus==UNAuthorizationStatusDenied) {
   dispatch_async(dispatch_get_main_queue(),^{goIOSSystemResult(token,"null",4,"");});
  } else submit();
 }];
}
void mygo_ios_remove_notification(const char *id) {
 NSArray *ids=@[string(id)];
 [UNUserNotificationCenter.currentNotificationCenter removePendingNotificationRequestsWithIdentifiers:ids];
 [UNUserNotificationCenter.currentNotificationCenter removeDeliveredNotificationsWithIdentifiers:ids];
 // APNs gives each request a system identifier. Resolve MyGo's stable logical
 // ID as well, so Close removes both local and remote reminders uniformly.
 NSString *identity=string(id);
 [UNUserNotificationCenter.currentNotificationCenter getDeliveredNotificationsWithCompletionHandler:^(NSArray<UNNotification *> *delivered){
  NSMutableArray *matches=[NSMutableArray array];
  for (UNNotification *n in delivered) if ([n.request.content.userInfo[@"mygoID"] isEqual:identity]) [matches addObject:n.request.identifier];
  if (matches.count) [UNUserNotificationCenter.currentNotificationCenter removeDeliveredNotificationsWithIdentifiers:matches];
 }];
}
void mygo_ios_clear_notifications(void) {
 [UNUserNotificationCenter.currentNotificationCenter removeAllPendingNotificationRequests];
 [UNUserNotificationCenter.currentNotificationCenter removeAllDeliveredNotifications];
}
void mygo_ios_badge(uint64_t token,int count) {
 UNUserNotificationCenter *center=UNUserNotificationCenter.currentNotificationCenter;
 [center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *s){
  dispatch_async(dispatch_get_main_queue(),^{
   if (s.badgeSetting!=UNNotificationSettingEnabled) {goIOSSystemResult(token,"null",4,"");return;}
   if (@available(iOS 16.0,*)) {
    [center setBadgeCount:count withCompletionHandler:^(NSError *error){dispatch_async(dispatch_get_main_queue(),^{result(token,error,2);});}];
   } else {UIApplication.sharedApplication.applicationIconBadgeNumber=count;result(token,nil,0);}
  });
 }];
}
void mygo_ios_register_push(void) { [UIApplication.sharedApplication registerForRemoteNotifications]; }
void mygo_ios_haptic(const char *kind) {
 static NSDictionary *generators;
 if (!generators) generators=@{
  @"selection":[UISelectionFeedbackGenerator new],@"success":[UINotificationFeedbackGenerator new],
  @"light":[[UIImpactFeedbackGenerator alloc] initWithStyle:UIImpactFeedbackStyleLight],
  @"medium":[[UIImpactFeedbackGenerator alloc] initWithStyle:UIImpactFeedbackStyleMedium],
  @"heavy":[[UIImpactFeedbackGenerator alloc] initWithStyle:UIImpactFeedbackStyleHeavy],
  @"soft":[[UIImpactFeedbackGenerator alloc] initWithStyle:UIImpactFeedbackStyleSoft],
  @"rigid":[[UIImpactFeedbackGenerator alloc] initWithStyle:UIImpactFeedbackStyleRigid]};
 NSString *k=string(kind);UIFeedbackGenerator *g=generators[k] ?: generators[@"success"];
 [g prepare];
 if ([k isEqual:@"selection"]) [(UISelectionFeedbackGenerator *)g selectionChanged];
 else if ([g isKindOfClass:UIImpactFeedbackGenerator.class]) [(UIImpactFeedbackGenerator *)g impactOccurred];
 else [(UINotificationFeedbackGenerator *)g notificationOccurred:[k isEqual:@"warning"]?UINotificationFeedbackTypeWarning:[k isEqual:@"error"]?UINotificationFeedbackTypeError:UINotificationFeedbackTypeSuccess];
}

int mygo_ios_badge_count(void) {return (int)UIApplication.sharedApplication.applicationIconBadgeNumber;}

void mygo_ios_dismiss_keyboard(void) {
 // Ends UIKit editing anywhere, including system text fields MyGo does not own.
 [UIApplication.sharedApplication sendAction:@selector(resignFirstResponder) to:nil from:nil forEvent:nil];
}
char *mygo_ios_device(void) {
 UIDevice *d=UIDevice.currentDevice;
 NSDictionary *info=@{@"Name":d.name ?: @"",@"System":d.systemName ?: @"",@"Version":d.systemVersion ?: @"",@"Model":d.model ?: @""};
 NSData *data=[NSJSONSerialization dataWithJSONObject:info options:0 error:nil];
 return strdup(((NSString *)[[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding]).UTF8String);
}

// A Foundation request makes iOS ask for local-network or cellular access and
// brings up VPN/DNS64 routes; BSD sockets opened first can fail silently.
static NSMutableDictionary<NSNumber *, NSURLSessionDataTask *> *networkTasks;
void mygo_ios_network(uint64_t token, const char *url, double timeout) {
 NSURL *u=[NSURL URLWithString:string(url)];
 if (!u) {goIOSSystemResult(token,"null",2,"mygo: invalid network preparation URL");return;}
 if (!networkTasks) networkTasks=[NSMutableDictionary dictionary];
 NSMutableURLRequest *request=[NSMutableURLRequest requestWithURL:u];
 request.HTTPMethod=@"HEAD";
 request.timeoutInterval=timeout>0 ? timeout : 10;
 NSURLSessionDataTask *task=[NSURLSession.sharedSession dataTaskWithRequest:request completionHandler:^(NSData *data,NSURLResponse *response,NSError *error){
  dispatch_async(dispatch_get_main_queue(),^{
   if (!networkTasks[@(token)]) return;
   [networkTasks removeObjectForKey:@(token)];
   // Any HTTP response proves the route; only transport failures matter.
   const char *kind="";
   if (error) {
    kind="failed";
    if ([error.domain isEqual:NSURLErrorDomain]) {
     if (error.code==NSURLErrorNotConnectedToInternet || error.code==NSURLErrorDataNotAllowed || error.code==NSURLErrorInternationalRoamingOff) kind="offline";
     else if (error.code==NSURLErrorTimedOut) kind="timeout";
    }
   }
   goIOSSystemResult(token,"null",error ? 8 : 0,(char *)kind);
  });
 }];
 networkTasks[@(token)]=task;
 [task resume];
}
void mygo_ios_network_cancel(uint64_t token) {
 NSURLSessionDataTask *task=networkTasks[@(token)];
 if (!task) return;
 [networkTasks removeObjectForKey:@(token)];
 [task cancel];
 goIOSSystemResult(token,"null",8,"canceled");
}
