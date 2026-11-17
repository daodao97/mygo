package dev.mygo

import com.android.billingclient.api.AcknowledgePurchaseParams
import com.android.billingclient.api.BillingClient
import com.android.billingclient.api.BillingClientStateListener
import com.android.billingclient.api.BillingFlowParams
import com.android.billingclient.api.BillingResult
import com.android.billingclient.api.ConsumeParams
import com.android.billingclient.api.PendingPurchasesParams
import com.android.billingclient.api.ProductDetails
import com.android.billingclient.api.Purchase
import com.android.billingclient.api.PurchasesUpdatedListener
import com.android.billingclient.api.QueryProductDetailsParams
import com.android.billingclient.api.QueryPurchasesParams
import org.json.JSONArray
import org.json.JSONObject

class PurchasesPlugin : MyGoPlugin {
    private var buying: PluginReply? = null
    private val details = HashMap<String, ProductDetails>()

    private val updated = PurchasesUpdatedListener { r, list ->
        val reply = buying
        buying = null
        when {
            r.responseCode == BillingClient.BillingResponseCode.USER_CANCELED -> reply?.failure(PluginFailure("Canceled"))
            r.responseCode != BillingClient.BillingResponseCode.OK -> reply?.failure(PluginFailure("Failed", r.debugMessage))
            else -> list?.forEachIndexed { i, p ->
                if (i == 0 && reply != null) reply.success(encode(p)) else PluginHost.emit("purchases", "purchase", encode(p))
            }
        }
    }

    private val client: BillingClient by lazy {
        BillingClient.newBuilder(PluginHost.context).setListener(updated)
            .enablePendingPurchases(PendingPurchasesParams.newBuilder().enableOneTimeProducts().build()).build()
    }

    private fun encode(p: Purchase) = JSONObject().put("productId", p.products.firstOrNull() ?: "").put("token", p.purchaseToken)
        .put("pending", p.purchaseState == Purchase.PurchaseState.PENDING).put("finished", p.isAcknowledged)

    private fun ready(reply: PluginReply, then: () -> Unit) {
        if (client.isReady) return then()
        client.startConnection(object : BillingClientStateListener {
            override fun onBillingServiceDisconnected() {}
            override fun onBillingSetupFinished(r: BillingResult) {
                if (r.responseCode == BillingClient.BillingResponseCode.OK) then() else reply.failure(PluginFailure("Failed", r.debugMessage))
            }
        })
    }

    private fun query(ids: List<String>, type: String, done: (List<ProductDetails>) -> Unit) {
        val params = QueryProductDetailsParams.newBuilder().setProductList(ids.map {
            QueryProductDetailsParams.Product.newBuilder().setProductId(it).setProductType(type).build()
        }).build()
        client.queryProductDetailsAsync(params) { _, list -> done(list) }
    }

    override fun call(method: String, args: JSONObject, reply: PluginReply) = ready(reply) {
        when (method) {
            "products" -> {
                val a = args.optJSONArray("ids") ?: JSONArray()
                val ids = List(a.length()) { a.getString(it) }
                query(ids, BillingClient.ProductType.INAPP) { one ->
                    query(ids, BillingClient.ProductType.SUBS) { subs ->
                        val out = JSONArray()
                        for (d in one + subs) {
                            details[d.productId] = d
                            val once = d.oneTimePurchaseOfferDetails
                            val sub = d.subscriptionOfferDetails?.firstOrNull()?.pricingPhases?.pricingPhaseList?.lastOrNull()
                            out.put(JSONObject().put("id", d.productId).put("title", d.name).put("description", d.description)
                                .put("price", once?.formattedPrice ?: sub?.formattedPrice ?: "")
                                .put("priceMicros", once?.priceAmountMicros ?: sub?.priceAmountMicros ?: 0)
                                .put("currency", once?.priceCurrencyCode ?: sub?.priceCurrencyCode ?: "")
                                .put("subscription", sub != null))
                        }
                        reply.success(JSONObject().put("products", out))
                    }
                }
            }
            "buy" -> {
                val a = PluginHost.activity
                val d = details[args.optString("id")]
                if (a == null || d == null) {
                    reply.failure(PluginFailure("NoProduct", "ask for its product first"))
                    return@ready
                }
                val offer = BillingFlowParams.ProductDetailsParams.newBuilder().setProductDetails(d)
                d.subscriptionOfferDetails?.firstOrNull()?.let { offer.setOfferToken(it.offerToken) }
                buying = reply
                client.launchBillingFlow(a, BillingFlowParams.newBuilder().setProductDetailsParamsList(listOf(offer.build())).build())
            }
            "finish" -> {
                val token = args.optString("token")
                if (args.optBoolean("consumable")) {
                    client.consumeAsync(ConsumeParams.newBuilder().setPurchaseToken(token).build()) { r, _ ->
                        if (r.responseCode == BillingClient.BillingResponseCode.OK) reply.success(null) else reply.failure(PluginFailure("Failed", r.debugMessage))
                    }
                } else {
                    client.acknowledgePurchase(AcknowledgePurchaseParams.newBuilder().setPurchaseToken(token).build()) { r ->
                        if (r.responseCode == BillingClient.BillingResponseCode.OK) reply.success(null) else reply.failure(PluginFailure("Failed", r.debugMessage))
                    }
                }
            }
            "restore" -> {
                val out = JSONArray()
                val q = { type: String, next: () -> Unit ->
                    client.queryPurchasesAsync(QueryPurchasesParams.newBuilder().setProductType(type).build()) { _, list ->
                        list.forEach { out.put(encode(it)) }
                        next()
                    }
                }
                q(BillingClient.ProductType.INAPP) { q(BillingClient.ProductType.SUBS) { reply.success(JSONObject().put("purchases", out)) } }
            }
            else -> reply.failure(PluginFailure("NoMethod", method))
        }
    }
}
