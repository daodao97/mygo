import BackgroundTasks
import UIKit

/// BGTaskScheduler: one identifier for what takes seconds and one for what takes
/// minutes, which the plugin's named tasks share, each with when it's due.
final class BackgroundPlugin: MyGoPlugin {
    private struct Task: Codable {
        var every: Double
        var delay: Double
        var next: Date
        var long: Bool
        var network: Bool
        var charging: Bool

        func sameOptions(as o: Task) -> Bool {
            every == o.every && delay == o.delay && long == o.long && network == o.network && charging == o.charging
        }
    }

    /// What one of the system's tasks is running.
    private final class Run {
        let task: BGTask
        let long: Bool
        var left: Set<Int>
        var ok = true
        var done = false
        init(task: BGTask, long: Bool, left: Set<Int>) {
            self.task = task
            self.long = long
            self.left = left
        }
    }

    private let refreshID: String
    private let processingID: String
    private let storeKey = "mygo.background.tasks"
    private var nextRun = 1
    private var runs: [Int: Run] = [:]
    private var names: [Int: String] = [:]
    private var ready = false
    private var waiting: [[String: Any]] = []

    init() {
        let id = Bundle.main.bundleIdentifier ?? ""
        refreshID = id + ".mygo.refresh"
        processingID = id + ".mygo.processing"
        // Before the app finishes launching, which is why plugins register then.
        BGTaskScheduler.shared.register(forTaskWithIdentifier: refreshID, using: .main) { [weak self] t in
            self?.fired(t, long: false)
        }
        BGTaskScheduler.shared.register(forTaskWithIdentifier: processingID, using: .main) { [weak self] t in
            self?.fired(t, long: true)
        }
    }

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        switch method {
        case "ready":
            ready = true
            for e in waiting { PluginHost.emit("background", "run", e) }
            waiting = []
            reply(.success(nil))
        case "schedule":
            var all = load()
            let name = args["name"] as? String ?? ""
            let every = (args["every"] as? Double) ?? 0
            let delay = (args["delay"] as? Double) ?? 0
            let t = Task(every: every, delay: delay, next: Date().addingTimeInterval(delay),
                         long: args["long"] as? Bool ?? false, network: args["network"] as? Bool ?? false,
                         charging: args["charging"] as? Bool ?? false)
            // The same again, as each launch asks: when it's due stands, or it
            // would be put off for ever by an app that's opened often.
            if let old = all[name], old.sameOptions(as: t) {
                reply(.success(nil))
                return
            }
            all[name] = t
            save(all)
            do {
                try submit(all)
                reply(.success(nil))
            } catch {
                // The simulator has no scheduler.
                reply(.failure(PluginFailure("Unavailable", "\(error)")))
            }
        case "cancel":
            var all = load()
            all[args["name"] as? String ?? ""] = nil
            save(all)
            try? submit(all)
            reply(.success(nil))
        case "done":
            finished(run: args["run"] as? Int ?? 0, ok: args["ok"] as? Bool ?? false)
            reply(.success(nil))
        default:
            reply(.failure(PluginFailure("UnknownMethod", method)))
        }
    }

    private func load() -> [String: Task] {
        guard let d = UserDefaults.standard.data(forKey: storeKey),
              let all = try? JSONDecoder().decode([String: Task].self, from: d) else { return [:] }
        return all
    }

    private func save(_ all: [String: Task]) {
        if let d = try? JSONEncoder().encode(all) { UserDefaults.standard.set(d, forKey: storeKey) }
    }

    /// Asks for the earliest of what's due, of each kind.
    private func submit(_ all: [String: Task]) throws {
        let quick = all.values.filter { !$0.long }
        let long = all.values.filter { $0.long }
        BGTaskScheduler.shared.cancel(taskRequestWithIdentifier: refreshID)
        BGTaskScheduler.shared.cancel(taskRequestWithIdentifier: processingID)
        if let at = quick.map(\.next).min() {
            let r = BGAppRefreshTaskRequest(identifier: refreshID)
            r.earliestBeginDate = at
            try BGTaskScheduler.shared.submit(r)
        }
        if let at = long.map(\.next).min() {
            let r = BGProcessingTaskRequest(identifier: processingID)
            r.earliestBeginDate = at
            r.requiresNetworkConnectivity = long.contains { $0.network }
            r.requiresExternalPower = long.contains { $0.charging }
            try BGTaskScheduler.shared.submit(r)
        }
    }

    private func fired(_ task: BGTask, long: Bool) {
        var all = load()
        let now = Date()
        let due = all.filter { $0.value.long == long && $0.value.next <= now.addingTimeInterval(60) }
        for (name, t) in due where t.every > 0 {
            all[name]?.next = now.addingTimeInterval(t.every)
        }
        save(all)
        try? submit(all)
        if due.isEmpty {
            task.setTaskCompleted(success: true)
            return
        }
        var ids = Set<Int>()
        for name in due.keys {
            let id = nextRun
            nextRun += 1
            ids.insert(id)
            names[id] = name
            let e: [String: Any] = ["run": id, "name": name]
            if ready {
                PluginHost.emit("background", "run", e)
            } else {
                waiting.append(e)
            }
        }
        let run = Run(task: task, long: long, left: ids)
        for id in ids { runs[id] = run }
        task.expirationHandler = { [weak self, weak run] in
            guard let self, let run else { return }
            // Handlers get a moment to let go of what they hold, as a database's
            // lock, before the system suspends the app.
            for id in run.left {
                if let i = self.waiting.firstIndex(where: { $0["run"] as? Int == id }) {
                    self.waiting.remove(at: i)
                    self.finished(run: id, ok: false)
                } else {
                    PluginHost.emit("background", "stop", ["run": id])
                }
            }
            DispatchQueue.main.asyncAfter(deadline: .now() + 2) { [weak self, weak run] in
                guard let self, let run, !run.done else { return }
                for id in run.left { self.finished(run: id, ok: false) }
            }
        }
    }

    private func finished(run id: Int, ok: Bool) {
        guard let run = runs.removeValue(forKey: id) else { return }
        run.left.remove(id)
        run.ok = run.ok && ok
        if ok, let name = names[id] {
            // Once it succeeded, what runs once is off the list; failed, it
            // stays, to run again.
            var all = load()
            if let t = all[name], t.every == 0 {
                all[name] = nil
                save(all)
                try? submit(all)
            }
        }
        names[id] = nil
        if run.left.isEmpty && !run.done {
            run.done = true
            run.task.setTaskCompleted(success: run.ok)
        }
    }
}
