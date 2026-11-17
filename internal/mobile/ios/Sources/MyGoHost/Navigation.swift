// The navigation of phones: each window in a view controller of its own,
// in a navigation controller (a stack, whose bar shows once a window is
// pushed), the navigation controllers of a bar of tabs in a tab bar
// controller, all children of the root view controller, the one shown
// last in front. A window's router may show its pages as screens of the
// stack (SetStack), one view controller each past the window's: the window
// moves into the one shown, and the others hold a snapshot of their page
// as it last showed, as Hotwire Native does with its web view.

import UIKit

/// A screen of a stack, which holds the window while it shows, and else
/// may hold a snapshot of what it showed.
class ScreenController: UIViewController, UIScrollViewDelegate {
    let window: HostWindow
    /// snapshot shows the page as it last showed, over the window, or in
    /// place of it.
    var snapshot: UIView?
    /// titles is the scroll view UIKit takes for the page's content, over
    /// the window, in which iOS 26 shows a large title.
    var titles: UIScrollView?

    init(_ w: HostWindow) {
        window = w
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) { fatalError() }

    override func loadView() {
        let v = UIView(frame: window.frame)
        v.backgroundColor = window.backgroundColor
        view = v
        if window is SurfaceWindow {
            // Native UI's scrolls are its own: this one, empty, clear and
            // over the window, which nothing scrolls, stands for them as the
            // page's content (contentScrollView). iOS 26 shows a large title
            // in it, at its top, and moves it with the page under the one a
            // finger takes back, beneath it; without one, it shows the title
            // at once, over that page.
            let t = UIScrollView(frame: v.bounds)
            t.autoresizingMask = [.flexibleWidth, .flexibleHeight]
            t.isUserInteractionEnabled = false
            // The status bar's tap goes to the one scroll view that takes it,
            // the root's, which tells Go.
            t.scrollsToTop = false
            t.backgroundColor = .clear
            t.delegate = self
            v.addSubview(t)
            setContentScrollView(t, for: .top)
            titles = t
        }
    }

    /// add puts a view of the page in it, under the large title.
    func add(_ sub: UIView) {
        if let t = titles { view.insertSubview(sub, belowSubview: t) } else { view.addSubview(sub) }
    }

    /// topInset is how far down the bars and a large title cover the page.
    var topInset: CGFloat? {
        guard let t = titles else { return nil }
        return t.adjustedContentInset.top
    }

    func scrollViewDidChangeAdjustedContentInset(_ s: UIScrollView) {
        if window.superview === view { window.reportSize() }
    }

    override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        if window.superview === view { window.reportSize() }
    }
}

/// The view controller of a window, whose view holds the window.
final class WindowController: ScreenController {
    /// barAtRoot shows the navigation bar at the bottom of a stack, as a
    /// tab's, with the window's title.
    var barAtRoot = false
    /// routed is set while the window's router shows its pages as screens
    /// of the stack, this the first: the bar shows its title.
    var routed = false

    override func loadView() {
        super.loadView()
        window.frame = view.bounds
        window.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        view.insertSubview(window, at: 0)
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        guard let nav = navigationController else { return }
        let atRoot = nav.viewControllers.first === self
        nav.setNavigationBarHidden(atRoot && !barAtRoot && !routed, animated: animated)
    }
}

/// A page of a window's router past its first, a screen of its stack.
final class RouteController: ScreenController {
    let key: String

    init(_ w: HostWindow, key: String, title: String) {
        self.key = key
        super.init(w)
        self.title = title
        // As pushed windows: the bar's small title.
        navigationItem.largeTitleDisplayMode = .never
    }

    required init?(coder: NSCoder) { fatalError() }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        navigationController?.setNavigationBarHidden(false, animated: animated)
    }
}

final class Navigation: NSObject, UINavigationControllerDelegate, UITabBarControllerDelegate {
    static let shared = Navigation()

    private var controllers: [Int: WindowController] = [:]
    /// The windows of each stack, as it last showed them, to tell which
    /// went (Popped).
    private var stacks: [ObjectIdentifier: [Int]] = [:]
    var tabs: [Int: UITabBarController] = [:]
    private var tabIDs: [ObjectIdentifier: Int] = [:]
    /// The screens of each window's router past its first (SetStack).
    private var routes: [Int: [RouteController]] = [:]
    /// moves counts each window's moves from screen to screen, which a
    /// reveal that comes late must not follow.
    private var moves: [Int: Int] = [:]
    private var root: RootViewController { Host.shared.root }

    func controller(_ w: HostWindow) -> WindowController {
        if let c = controllers[w.id] { return c }
        let c = WindowController(w)
        controllers[w.id] = c
        return c
    }

    func forget(_ w: HostWindow) {
        controllers[w.id] = nil
        routes[w.id] = nil
        moves[w.id] = nil
    }

    private func stack(_ root: UIViewController) -> UINavigationController {
        let nav = UINavigationController(rootViewController: root)
        nav.delegate = self
        nav.navigationBar.prefersLargeTitles = true
        stacks[ObjectIdentifier(nav)] = [(root as? WindowController)?.window.id ?? 0]
        return nav
    }

    /// add puts a child view controller of the root in front.
    private func add(_ vc: UIViewController) {
        if vc.parent !== root {
            vc.willMove(toParent: nil)
            vc.view.removeFromSuperview()
            vc.removeFromParent()
            root.addChild(vc)
            vc.view.frame = root.view.bounds
            vc.view.autoresizingMask = [.flexibleWidth, .flexibleHeight]
            root.view.addSubview(vc.view)
            vc.didMove(toParent: root)
        } else {
            root.view.bringSubviewToFront(vc.view)
        }
    }

    private func remove(_ vc: UIViewController) {
        vc.willMove(toParent: nil)
        vc.view.removeFromSuperview()
        vc.removeFromParent()
    }

    /// show brings a window to the front: its tab, its stack, or a stack of
    /// its own.
    func show(_ w: HostWindow) {
        let c = controller(w)
        if let nav = c.navigationController {
            if let tbc = nav.tabBarController {
                tbc.selectedViewController = nav
                add(tbc)
            } else {
                add(nav)
            }
            return
        }
        add(stack(c))
    }

    /// hide takes a window off the screen: its stack, or the windows above
    /// it in its stack.
    func hide(_ w: HostWindow) {
        guard let c = controllers[w.id], let nav = c.navigationController else { return }
        if nav.viewControllers.first === c {
            if nav.tabBarController == nil {
                stacks[ObjectIdentifier(nav)] = nil
                remove(nav)
            }
        } else if let i = nav.viewControllers.firstIndex(of: c) {
            nav.setViewControllers(Array(nav.viewControllers[..<i]), animated: false)
        }
    }

    func push(_ w: HostWindow, over parent: HostWindow, title: String, animated: Bool) {
        let p = controller(parent)
        let nav = p.navigationController ?? {
            let n = stack(p)
            add(n)
            return n
        }()
        let c = controller(w)
        c.title = title
        // Pushed windows take the bar's small title, as Settings' pages.
        c.navigationItem.largeTitleDisplayMode = .never
        // Above the window it is pushed over, as it was pushed from it: above
        // the screens of its router.
        let over: UIViewController = routes[parent.id]?.last ?? p
        if let i = nav.viewControllers.firstIndex(of: over), i < nav.viewControllers.count - 1 {
            nav.setViewControllers(Array(nav.viewControllers[...i]), animated: false)
        }
        nav.pushViewController(c, animated: animated)
    }

    func pop(_ w: HostWindow, animated: Bool) {
        guard let c = controllers[w.id], let nav = c.navigationController,
              let i = nav.viewControllers.firstIndex(of: c), i > 0 else { return }
        nav.popToViewController(nav.viewControllers[i - 1], animated: animated)
    }

    func navigationController(_ nav: UINavigationController, didShow vc: UIViewController, animated: Bool) {
        // Screens of routers the user took off, with the back button or a
        // swipe: the window goes into the screen shown, and its router back.
        for (id, rs) in routes {
            guard !rs.isEmpty, controllers[id]?.navigationController === nav else { continue }
            let left = Array(rs.prefix { nav.viewControllers.contains($0) })
            if left.count == rs.count { continue }
            routes[id] = left
            let w = rs[0].window
            let target: ScreenController = left.last ?? controller(w)
            move(w, into: target, back: true)
            CATransaction.flush()
            Go.send(Event.stackPopped(id: id, depth: left.count + 1))
        }
        let ids = nav.viewControllers.compactMap { ($0 as? WindowController)?.window.id }
        let key = ObjectIdentifier(nav)
        for id in stacks[key] ?? [] where !ids.contains(id) {
            Go.send(Event.popped(id: id))
        }
        stacks[key] = ids
    }

    // The screens of routers.

    /// setStack shows the pages of w's router as screens of its stack: the
    /// window's own first, then a RouteController each. The screens of the
    /// keys it shows stay; a page deeper is pushed, one less deep popped
    /// back to, and others replace theirs at once. No titles take them away.
    func setStack(_ w: HostWindow, titles: [String], keys: [String]) {
        let wc = controller(w)
        guard let nav = wc.navigationController, let at = nav.viewControllers.firstIndex(of: wc) else { return }
        let old = routes[w.id] ?? []
        let base = Array(nav.viewControllers[...at])
        if titles.isEmpty {
            routes[w.id] = nil
            wc.routed = false
            wc.navigationItem.title = wc.title
            uncover(wc)
            move(w, into: wc)
            w.isHidden = false
            if !old.isEmpty { nav.setViewControllers(base, animated: false) }
            if nav.topViewController === wc {
                nav.setNavigationBarHidden(at == 0 && !wc.barAtRoot, animated: false)
            }
            return
        }
        if !wc.routed {
            wc.routed = true
            if nav.topViewController === wc { nav.setNavigationBarHidden(false, animated: false) }
        }
        wc.navigationItem.title = titles[0]
        let want = Array(zip(titles.dropFirst(), keys.dropFirst()))
        var common = 0
        while common < old.count, common < want.count, old[common].key == want[common].1 {
            old[common].title = want[common].0
            common += 1
        }
        if common == old.count, common == want.count { return } // titles alone
        let top: ScreenController = old.last ?? wc
        let fresh = want[common...].map { RouteController(w, key: $0.1, title: $0.0) }
        let now = Array(old[..<common]) + fresh
        routes[w.id] = now
        let target: ScreenController = now.last ?? wc
        let animated = nav.view.window != nil && (common == old.count || common == want.count)
        if animated {
            // The page going out of sight stays in its screen, as it last
            // showed: the surface shows the page coming next.
            cover(top, w)
        }
        if !fresh.isEmpty { uncover(target) }
        move(w, into: target, back: fresh.isEmpty)
        nav.setViewControllers(base + now, animated: animated)
        // On screen before Go presents the page coming: Metal's frames show
        // as they come, not with UIKit's next commit.
        CATransaction.flush()
    }

    /// stackShown shows the window over the snapshot of its screen, once Go
    /// presented its page there: a frame later, as the frame presents.
    func stackShown(_ w: HostWindow) {
        let n = moves[w.id] ?? 0
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.0 / 30) { [weak self, weak w] in
            guard let self, let w, (self.moves[w.id] ?? 0) == n,
                  let vc = w.superview?.next as? ScreenController else { return }
            self.uncover(vc)
            w.isHidden = false
        }
    }

    /// cover leaves in vc a snapshot of what the window shows, over it.
    private func cover(_ vc: ScreenController, _ w: HostWindow) {
        uncover(vc)
        guard w.superview === vc.view, !w.isHidden, let s = w.snapshotView(afterScreenUpdates: false) else { return }
        s.frame = w.frame
        s.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        vc.add(s)
        vc.snapshot = s
    }

    private func uncover(_ vc: ScreenController) {
        vc.snapshot?.removeFromSuperview()
        vc.snapshot = nil
    }

    /// move puts the window in vc, under its snapshot. Going back to a screen
    /// without one, it hides, as it shows another page: StackShown shows it.
    private func move(_ w: HostWindow, into vc: ScreenController, back: Bool = false) {
        guard w.superview !== vc.view else { return }
        moves[w.id, default: 0] += 1
        let from = w.superview
        let page = vc.view!
        if page.bounds.isEmpty, let from { page.frame = from.bounds }
        w.frame = page.bounds
        w.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        page.insertSubview(w, at: 0)
        w.isHidden = back && vc.snapshot == nil
    }

    // Tabs.

    func createTabs(_ id: Int) {
        let tbc = UITabBarController()
        tbc.delegate = self
        tbc.viewControllers = []
        tabs[id] = tbc
        tabIDs[ObjectIdentifier(tbc)] = id
    }

    func addTab(_ id: Int, _ w: HostWindow, title: String, symbol: String, icon: [UInt8]) {
        guard let tbc = tabs[id] else { return }
        let c = controller(w)
        if let old = c.navigationController {
            // Shown on its own before: the tab takes it.
            remove(old)
            old.setViewControllers([], animated: false)
        }
        c.title = title
        c.barAtRoot = !title.isEmpty
        let nav = stack(c)
        var image = symbol.isEmpty ? nil : UIImage(systemName: symbol)
        if image == nil, !icon.isEmpty {
            image = UIImage(data: Data(icon), scale: 3)?.withRenderingMode(.alwaysTemplate)
        }
        nav.tabBarItem = UITabBarItem(title: title, image: image, tag: tbc.viewControllers?.count ?? 0)
        tbc.viewControllers = (tbc.viewControllers ?? []) + [nav]
    }

    func showTabs(_ id: Int) {
        guard let tbc = tabs[id] else { return }
        add(tbc)
    }

    func closeTabs(_ id: Int) {
        guard let tbc = tabs.removeValue(forKey: id) else { return }
        tabIDs[ObjectIdentifier(tbc)] = nil
        remove(tbc)
    }

    func tabBarController(_ tbc: UITabBarController, didSelect vc: UIViewController) {
        guard let id = tabIDs[ObjectIdentifier(tbc)], let i = tbc.viewControllers?.firstIndex(of: vc) else { return }
        Go.send(Event.tabSelected(id: id, index: i))
    }
}

extension Array {
    subscript(safe i: Int) -> Element? { i >= 0 && i < count ? self[i] : nil }
}
