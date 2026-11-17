package dev.mygo

import org.json.JSONObject
import org.osmdroid.config.Configuration
import org.osmdroid.events.MapEventsReceiver
import org.osmdroid.tileprovider.tilesource.OnlineTileSourceBase
import org.osmdroid.tileprovider.tilesource.TileSourceFactory
import org.osmdroid.util.GeoPoint
import org.osmdroid.util.MapTileIndex
import org.osmdroid.views.CustomZoomButtonsController
import org.osmdroid.views.MapView
import org.osmdroid.views.overlay.MapEventsOverlay
import org.osmdroid.views.overlay.Marker
import java.io.File

/** A map's tiles from a server of the app's. */
private class Tiles(private val template: String) : OnlineTileSourceBase("mygo", 0, 22, 256, "", arrayOf(template)) {
    override fun getTileURLString(index: Long): String = template
        .replace("{z}", MapTileIndex.getZoom(index).toString())
        .replace("{x}", MapTileIndex.getX(index).toString())
        .replace("{y}", MapTileIndex.getY(index).toString())
}

class MapsPlugin : MyGoPlugin {
    private class Holder(val id: Long, val map: MapView) {
        val pins = HashMap<String, Marker>()
    }

    private val maps = HashMap<Long, Holder>()

    init {
        PluginHost.registerView("map") { context, id, args ->
            val conf = Configuration.getInstance()
            conf.userAgentValue = context.packageName
            conf.osmdroidBasePath = File(context.cacheDir, "osmdroid")
            conf.osmdroidTileCache = File(context.cacheDir, "osmdroid/tiles")
            val map = MapView(context)
            val template = args.optString("tileURL")
            map.setTileSource(if (template.isEmpty()) TileSourceFactory.MAPNIK else Tiles(template))
            map.setMultiTouchControls(true)
            map.isTilesScaledToDpi = true // legible on a dense screen
            map.zoomController.setVisibility(CustomZoomButtonsController.Visibility.NEVER)
            map.controller.setZoom(args.optDouble("zoom", 2.0))
            map.controller.setCenter(GeoPoint(args.optDouble("lat"), args.optDouble("lng")))
            val h = Holder(id, map)
            maps[id] = h
            map.overlays.add(MapEventsOverlay(object : MapEventsReceiver {
                override fun singleTapConfirmedHelper(p: GeoPoint): Boolean {
                    PluginHost.emit("maps", "tap", mapOf("id" to id, "lat" to p.latitude, "lng" to p.longitude))
                    return true
                }
                override fun longPressHelper(p: GeoPoint) = false
            }))
            map.onResume()
            map
        }
    }

    override fun call(method: String, args: JSONObject, reply: PluginReply) {
        val id = args.optLong("id")
        val h = maps[id]
        if (h == null) {
            reply.failure(PluginFailure("NoMap", "no map $id"))
            return
        }
        val at = GeoPoint(args.optDouble("lat"), args.optDouble("lng"))
        when (method) {
            "setCamera" -> {
                if (args.optBoolean("animated")) {
                    h.map.controller.animateTo(at, args.optDouble("zoom", h.map.zoomLevelDouble), null)
                } else {
                    h.map.controller.setZoom(args.optDouble("zoom", h.map.zoomLevelDouble))
                    h.map.controller.setCenter(at)
                }
                reply.success(null)
            }
            "camera" -> {
                val c = h.map.mapCenter
                reply.success(mapOf("Lat" to c.latitude, "Lng" to c.longitude, "Zoom" to h.map.zoomLevelDouble))
            }
            "addMarker" -> {
                val name = args.optString("marker")
                h.pins.remove(name)?.let {
                    it.closeInfoWindow()
                    h.map.overlays.remove(it)
                }
                val m = Marker(h.map)
                m.position = at
                m.title = args.optString("title").ifEmpty { null }
                m.setAnchor(Marker.ANCHOR_CENTER, Marker.ANCHOR_BOTTOM)
                m.setOnMarkerClickListener { marker, map ->
                    PluginHost.emit("maps", "markerTap", mapOf("id" to id, "marker" to name))
                    if (marker.title != null) marker.showInfoWindow()
                    map.controller.animateTo(marker.position)
                    true
                }
                h.pins[name] = m
                h.map.overlays.add(m)
                h.map.invalidate()
                reply.success(null)
            }
            "removeMarker" -> {
                h.pins.remove(args.optString("marker"))?.let {
                    it.closeInfoWindow()
                    h.map.overlays.remove(it)
                }
                h.map.invalidate()
                reply.success(null)
            }
            "dispose" -> {
                maps.remove(id)
                h.map.onPause()
                h.map.onDetach()
                reply.success(null)
            }
            else -> reply.failure(PluginFailure("UnknownMethod", method))
        }
    }
}
