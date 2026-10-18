//go:build ios && cgo
#import <Foundation/Foundation.h>
#import <Network/Network.h>
#include <stdatomic.h>
#include <stdint.h>
extern void mygo_ios_network_changed(uint32_t mask);
static atomic_uint pathMask;
static atomic_bool watching;
static nw_path_monitor_t monitor;
static void startPathMonitor(void) {
 static dispatch_once_t once;
 dispatch_once(&once, ^{
  monitor=nw_path_monitor_create();
  nw_path_monitor_set_update_handler(monitor, ^(nw_path_t path){
   uint32_t value=1;
   if(nw_path_get_status(path)==nw_path_status_satisfied)value|=2;
   if(nw_path_is_expensive(path))value|=4;
   if(nw_path_is_constrained(path))value|=8;
   if(nw_path_uses_interface_type(path,nw_interface_type_wifi))value|=16;
   if(nw_path_uses_interface_type(path,nw_interface_type_cellular))value|=32;
   if(nw_path_uses_interface_type(path,nw_interface_type_wired))value|=64;
   atomic_store(&pathMask,value);
   if(atomic_load(&watching))dispatch_async(dispatch_get_main_queue(), ^{mygo_ios_network_changed(value);});
  });
  nw_path_monitor_set_queue(monitor,dispatch_queue_create("org.mygo.network.path",DISPATCH_QUEUE_SERIAL));
  nw_path_monitor_start(monitor);
 });
}
uint32_t mygo_ios_network_status(void){startPathMonitor();return atomic_load(&pathMask);}
void mygo_ios_network_watch(void){atomic_store(&watching,true);startPathMonitor();}
