package dev.mygo

import android.content.Context
import android.content.pm.PackageManager
import java.io.File

/**
 * The app's resources (mygo.PathResources): mygo build puts them in the
 * APK's assets, under resources/, which are no files. They are copied
 * into the app's files once per version of the app, for Go to read them
 * by path.
 */
object Resources {
    fun dir(c: Context) = File(c.filesDir, "mygo-resources")

    fun extract(c: Context): String {
        val dir = dir(c)
        val info = c.packageManager.getPackageInfo(c.packageName, 0)
        val stamp = File(dir, ".version")
        val version = "${info.longVersionCode}-${info.lastUpdateTime}"
        if (stamp.exists() && stamp.readText() == version) return dir.absolutePath
        dir.deleteRecursively()
        dir.mkdirs()
        copy(c, "resources", dir)
        stamp.writeText(version)
        return dir.absolutePath
    }

    private fun copy(c: Context, path: String, to: File) {
        val names = c.assets.list(path) ?: return
        for (n in names) {
            val p = "$path/$n"
            val sub = c.assets.list(p)
            if (sub != null && sub.isNotEmpty()) {
                File(to, n).mkdirs()
                copy(c, p, File(to, n))
            } else {
                c.assets.open(p).use { input -> File(to, n).outputStream().use { input.copyTo(it) } }
            }
        }
    }
}
