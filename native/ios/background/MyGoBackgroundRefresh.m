#import <Foundation/Foundation.h>
#import <BackgroundTasks/BackgroundTasks.h>
#import <TargetConditionals.h>
#import "MyGoBackgroundRefresh.h"
#include <math.h>
#include <stdlib.h>
// An expiration can arrive off-main, even after completion was queued. Keep
// its flag per launch so it cannot cancel or mark a later launch expired.
@interface MyGoRefreshLaunch : NSObject
@property(atomic) BOOL expired;
@end
@implementation MyGoRefreshLaunch
@end
@interface MyGoRefreshJob : NSObject
@property(nonatomic,copy) NSString *identifier;
@property(nonatomic,copy) NSString *lastError;
@property(nonatomic) void (*prepare)(void);
@property(nonatomic) int (*run)(void);
@property(nonatomic) void (*cancel)(void);
@property(nonatomic) double (*nextDelay)(void);
@property(nonatomic) BOOL active;
@property(nonatomic,strong) MyGoRefreshLaunch *launch;
@property(nonatomic,strong) dispatch_queue_t scheduleQueue;
@end
@implementation MyGoRefreshJob
@end
static NSMutableDictionary<NSString *,MyGoRefreshJob *> *jobs;
static NSObject *registryLock(void) { static NSObject *lock; static dispatch_once_t once; dispatch_once(&once, ^{lock=[NSObject new]; jobs=[NSMutableDictionary new];}); return lock; }
static MyGoRefreshJob *jobFor(NSString *identifier) { @synchronized(registryLock()) {return jobs[identifier];} }
int MyGoBackgroundRefreshRegistered(const char *identifier) {
 @autoreleasepool { return jobFor([NSString stringWithUTF8String:identifier])!=nil; }
}
static void setError(MyGoRefreshJob *job,NSString *error) { @synchronized(job) { job.lastError=error?:@""; } }
char *MyGoBackgroundRefreshLastError(const char *identifier) {
 @autoreleasepool { MyGoRefreshJob *job=jobFor([NSString stringWithUTF8String:identifier]);if(!job)return strdup("Background task is not registered"); @synchronized(job){return strdup((job.lastError?:@"").UTF8String);} }
}
void MyGoScheduleBackgroundRefresh(const char *identifier) {
 @autoreleasepool {
  MyGoRefreshJob *job=jobFor([NSString stringWithUTF8String:identifier]);if(!job)return;
  if(TARGET_OS_SIMULATOR)return;
  dispatch_async(job.scheduleQueue, ^{
   @autoreleasepool {
    double delay=job.nextDelay();
    [BGTaskScheduler.sharedScheduler cancelTaskRequestWithIdentifier:job.identifier];
    if(delay<0){setError(job,@"");return;}
    if(!isfinite(delay)){setError(job,@"Invalid background refresh delay");return;}
    BGAppRefreshTaskRequest *request=[[BGAppRefreshTaskRequest alloc] initWithIdentifier:job.identifier];
    request.earliestBeginDate=[NSDate dateWithTimeIntervalSinceNow:MAX(0,delay)];
    NSError *error=nil;
    BOOL ok=[BGTaskScheduler.sharedScheduler submitTaskRequest:request error:&error];
    setError(job,ok?@"":error.localizedDescription?:@"Background refresh scheduling failed");
   }
  });
 }
}
void MyGoRegisterBackgroundRefresh(const char *identifier,void (*prepare)(void),int (*run)(void),void (*cancel)(void),double (*nextDelay)(void)) {
 @autoreleasepool {
  NSCAssert(NSThread.isMainThread,@"Register background tasks before UIApplicationMain on the main thread");
  NSString *name=[NSString stringWithUTF8String:identifier];
  MyGoRefreshJob *job=[MyGoRefreshJob new];job.identifier=name;job.prepare=prepare;job.run=run;job.cancel=cancel;job.nextDelay=nextDelay;
  job.scheduleQueue=dispatch_queue_create("org.mygo.background.schedule",DISPATCH_QUEUE_SERIAL);
  @synchronized(registryLock()){if(jobs[name])return;jobs[name]=job;}
  if(TARGET_OS_SIMULATOR)return;
  BOOL registered=[BGTaskScheduler.sharedScheduler registerForTaskWithIdentifier:name usingQueue:dispatch_get_main_queue() launchHandler:^(__kindof BGTask *task){
   if(job.active){[task setTaskCompletedWithSuccess:NO];return;}
   job.active=YES;
   MyGoRefreshLaunch *launch=[MyGoRefreshLaunch new];job.launch=launch;
   job.prepare();
   task.expirationHandler=^{
    launch.expired=YES;
    dispatch_async(dispatch_get_main_queue(), ^{
     if(job.active && job.launch==launch)job.cancel();
    });
   };
   dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY,0), ^{
    @autoreleasepool {
     BOOL success=job.run()!=0;
     dispatch_async(dispatch_get_main_queue(), ^{
      task.expirationHandler=nil;[task setTaskCompletedWithSuccess:success&&!launch.expired];job.active=NO;job.launch=nil;
      MyGoScheduleBackgroundRefresh(job.identifier.UTF8String);
     });
    }
   });
  }];
  if(!registered)setError(job,@"Background refresh registration failed");
 }
}
