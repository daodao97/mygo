# JNI reaches these by name.
-keep class dev.mygo.Host { *; }
-keep class dev.mygo.MyGoNative { *; }
-keep class dev.mygo.Native { *; }
# Go loads a native module's Kotlin by its generated object's class.
-keep class * extends dev.mygo.NativeModule { public static ** INSTANCE; }
-keepclassmembers class * { @android.webkit.JavascriptInterface <methods>; }
