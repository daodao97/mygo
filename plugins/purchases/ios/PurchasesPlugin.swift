import Foundation
import StoreKit

final class PurchasesPlugin: MyGoPlugin {
    private var open: [String: Transaction] = [:]
    private var updates: Task<Void, Never>?

    init() {
        updates = Task { [weak self] in
            for await result in Transaction.updates {
                if case .verified(let t) = result, let self {
                    await MainActor.run { self.open[t.productID] = t }
                    PluginHost.emit("purchases", "purchase", ["productId": t.productID, "token": result.jwsRepresentation, "pending": false, "finished": false])
                }
            }
        }
    }

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        Task { @MainActor in
            do {
                switch method {
                case "products":
                    let list = try await Product.products(for: args["ids"] as? [String] ?? [])
                    reply(.success(["products": list.map { p -> [String: Any] in
                        ["id": p.id, "title": p.displayName, "description": p.description, "price": p.displayPrice,
                         "priceMicros": Int64((p.price as NSDecimalNumber).doubleValue * 1_000_000),
                         "currency": p.priceFormatStyle.currencyCode, "subscription": p.subscription != nil]
                    }]))
                case "buy":
                    guard let p = try await Product.products(for: [args["id"] as? String ?? ""]).first else {
                        return reply(.failure(PluginFailure("NoProduct", "no such product")))
                    }
                    switch try await p.purchase() {
                    case .success(let result):
                        guard case .verified(let t) = result else {
                            return reply(.failure(PluginFailure("Unverified", "the store's signature does not verify")))
                        }
                        self.open[t.productID] = t
                        reply(.success(["productId": t.productID, "token": result.jwsRepresentation, "pending": false, "finished": false]))
                    case .pending:
                        reply(.success(["productId": p.id, "token": "", "pending": true, "finished": false]))
                    case .userCancelled:
                        reply(.failure(PluginFailure("Canceled")))
                    @unknown default:
                        reply(.failure(PluginFailure("Failed", "the store answered something unknown")))
                    }
                case "finish":
                    let id = args["productId"] as? String ?? ""
                    await self.open[id]?.finish()
                    self.open[id] = nil
                    reply(.success(nil))
                case "restore":
                    try? await AppStore.sync()
                    var out: [[String: Any]] = []
                    for await result in Transaction.currentEntitlements {
                        if case .verified(let t) = result {
                            out.append(["productId": t.productID, "token": result.jwsRepresentation, "pending": false, "finished": true])
                        }
                    }
                    reply(.success(["purchases": out]))
                default:
                    reply(.failure(PluginFailure("NoMethod", method)))
                }
            } catch {
                reply(.failure(PluginFailure("Failed", error.localizedDescription)))
            }
        }
    }
}
