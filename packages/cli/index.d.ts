/** The platforms with a prebuilt binary, as `${process.platform}-${process.arch}`. */
export declare const platforms: readonly string[];

/**
 * Returns the name of the package holding the mygo binary of a platform,
 * by default this one. Throws for platforms without a prebuilt binary.
 */
export declare function platformPackage(platform?: string, arch?: string): string;

/**
 * Returns the path of the mygo binary to run: $MYGO_CLI_BINARY when set,
 * else the binary of this platform's package. Throws when that package is
 * not installed.
 */
export declare function binaryPath(): string;

/**
 * Types the configuration of mygo.config.ts, which takes the place of
 * mygo.json:
 *
 * ```ts
 * import { defineConfig } from "mygo-cli";
 *
 * export default defineConfig({
 *   name: "My App",
 *   identifier: "com.example.myapp",
 *   devUrl: "http://localhost:5173",
 * });
 * ```
 *
 * A function gets the command running and returns the configuration, or a
 * promise of it.
 */
export declare function defineConfig(config: Config): Config;
export declare function defineConfig(config: Promise<Config>): Promise<Config>;
export declare function defineConfig(config: ConfigFn): ConfigFn;

/** A configuration computed for the command running. */
export type ConfigFn = (env: ConfigEnv) => Config | Promise<Config>;

/** What a configuration function gets. */
export interface ConfigEnv {
  /** The mygo command running. */
  command: "dev" | "build" | "generate" | "init" | "publish";
}

/** The configuration of a MyGo project. Every field is optional. */
export interface Config {
  /** The name users see: the app, its executable, menus and installers (default: the directory's name). */
  name?: string;
  /**
   * The name of the app's files where a name must be plain ASCII: the disk
   * images of macOS and the installers of Windows, the Linux executable,
   * desktop entry, icon and Debian package, the update archives of every
   * platform, and the variables of the install scripts. Lower-case
   * letters and digits with single hyphens between them (default: the name
   * in lower case, with hyphens between its words and Chinese, Japanese and
   * Korean in Latin letters, as `my-app` of "My App" and `yun-yin-le` of
   * "云音乐").
   */
  slug?: string;
  /**
   * A reverse DNS name unique to the app, such as `com.example.myapp`, on
   * which systems key preferences, permissions and registrations (default:
   * `com.mygo.` and the name's letters and digits).
   */
  identifier?: string;
  /** The version of the app, which updates compare (default: `0.1.0`). */
  version?: string;
  /** The copyright notice of the app. */
  copyright?: string;
  /**
   * Who makes the app, which Windows lists with the installed app (default:
   * the copyright notice without its "©" and years).
   */
  publisher?: string;
  /**
   * The SPDX license expression of the app, such as `MIT`, which the RPM
   * package and the PKGBUILD of Arch Linux record (default: `license` in
   * `package.json`).
   */
  license?: string;
  /** A square PNG, ideally 1024×1024 (default: `resources/icon.png` when it exists). */
  icon?: string;
  /** The Go package of the app, relative to the project (default: `.`). */
  main?: string;
  /** Where `mygo build` writes the builds, one directory per platform (default: `dist`). */
  out?: string;
  /**
   * Where `mygo generate` writes the TypeScript client (default:
   * `src/mygo.ts` when package.json at the project root depends on
   * mygo-runtime; an app without a frontend gets no client).
   */
  bindings?: string;

  /**
   * The dev server `mygo dev` points the app at, such as
   * `http://localhost:5173`: windows load it for URLs without a scheme.
   */
  devUrl?: string;
  /** Runs in the project while `mygo dev` runs, e.g. the dev server; the app starts once `devUrl` answers. */
  devCommand?: string;
  /** Builds the frontend before `mygo build` compiles the app. */
  buildCommand?: string;
  /**
   * The directory of the built frontend, which `mygo build` embeds into the
   * app; `mygo dev` serves it from disk without `devUrl`.
   */
  frontendDist?: string;

  /**
   * Files and directories copied into the app of every platform under their
   * base names, next to the contents of the project's resources directory,
   * whose directories named after a platform, such as
   * `resources/linux-amd64`, hold the files of one platform.
   */
  resources?: string[];
  /**
   * Files of the project's resources directory left out of the apps:
   * patterns of their paths there, with slashes, where `*` matches within a
   * name and `**` any number of directories, as in `design/**`. A directory
   * that matches is left out with what it holds. Go source files are always
   * left out; listed `resources` ship whole.
   */
  resourcesExclude?: string[];
  /** The URL schemes the app opens, such as `myapp` for `myapp://…`, which reach `App.OnOpenURL`. */
  urlSchemes?: string[];
  /** The types of files the app opens, which reach `App.OnOpenFile`. */
  fileAssociations?: FileAssociation[];
  /**
   * The catalogs of the app's messages, which make the languages it ships.
   * Without it, the app ships English only.
   */
  i18n?: I18nConfig;
  /** Signed updates, which `mygo.Updater` installs. */
  updates?: UpdatesConfig;
  /** macOS packaging. */
  macos?: MacOSConfig;
  /** Windows packaging. */
  windows?: WindowsConfig;
  /** Linux packaging. */
  linux?: LinuxConfig;
  /** The iOS app (`mygo build -platform ios`, `ios/simulator`). */
  ios?: IOSConfig;
  /** The Android app (`mygo build -platform android`). */
  android?: AndroidConfig;
}

/**
 * The catalogs of the app's messages: a JSON file per language in `dir`,
 * named after its BCP 47 tag (`de.json`, `zh-Hant.json`), whose messages
 * `mygo generate` writes typed Go and TypeScript functions for.
 */
export interface I18nConfig {
  /** The directory of the catalogs, a Go package of its own (default `locales`). */
  dir?: string;
  /** The language the app is written in, which the others fall back to (default `en`). */
  source?: string;
}

/** A type of file the app opens. */
export interface FileAssociation {
  /** The file name extensions, without dots, e.g. `["md"]`. */
  ext: string[];
  /** What the files are, e.g. `"Markdown Document"`. */
  name?: string;
  /** The MIME type of the files, by which Linux identifies them; the app defines an unregistered one (`x-`) (default: `application/x-<app>-<ext>`). */
  mimeType?: string;
  /** The app's role for the files on macOS (default: `"Editor"`). */
  role?: "Editor" | "Viewer";
}

/** Signed updates: `publicKey` and one of `github` and `url` are required. */
export interface UpdatesConfig {
  /** The contents of mygo-update.pub, from `mygo keygen`. */
  publicKey: string;
  /** A public GitHub repository, `owner/name`, whose releases hold the updates. */
  github?: string;
  /**
   * What precedes the version in release tags (default: `v`). Another prefix,
   * such as `desktop-v`, lets the repository hold other releases: apps find
   * the newest release tagged with it through the GitHub API instead of
   * reading the latest release.
   */
  tagPrefix?: string;
  /** Instead of `github`, the HTTPS URL of a directory holding the updates. */
  url?: string;
  /** The bucket that serves `url`, which `mygo build -upload` uploads to. */
  s3?: S3Config;
  /** The path of mygo-update.key; `MYGO_UPDATER_PRIVATE_KEY`, holding the key, takes precedence. */
  privateKey?: string;
  /** A Markdown file whose `## <version>` section becomes the release notes (default: `CHANGELOG.md` when it exists). */
  changelog?: string;
  /** How many earlier versions get a delta update, a smaller download of what changed (default: `3`; `0` for none). */
  deltas?: number;
}

/**
 * A bucket of Amazon S3, or of a compatible service such as Cloudflare R2.
 * The credentials come from `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` and
 * `AWS_SESSION_TOKEN`.
 */
export interface S3Config {
  /** The name of the bucket. */
  bucket: string;
  /** The directory of the files in the bucket, e.g. `my-app` (default: its root). */
  prefix?: string;
  /** The region of the bucket (default: `AWS_REGION`, else `AWS_DEFAULT_REGION`, else `us-east-1`). */
  region?: string;
  /** The endpoint of a compatible service, e.g. `https://<account>.r2.cloudflarestorage.com` (default: Amazon S3's). */
  endpoint?: string;
  /** Names the bucket in the path of URLs rather than in the host name (default: with `endpoint`, or a bucket name with dots). */
  pathStyle?: boolean;
}

/** macOS packaging. */
export interface MacOSConfig {
  /**
   * The icon as an asset catalog: an `Assets.car` compiled with actool (its
   * app icon named `AppIcon`, unless `infoPlist` sets `CFBundleIconName`), or
   * an Icon Composer `.icon`, which builds compile on a Mac with Xcode 26 or
   * later and skip elsewhere. The `.icns` made from `icon` remains the
   * fallback. Or an `.icns` in place of the one made from `icon`, for an
   * icon drawn per size.
   */
  icon?: string;
  /** The oldest macOS the app runs on (default: `12.0`). */
  minimumSystemVersion?: string;
  /** The identity that signs the app and its disk image, e.g. `Developer ID Application: Jane Doe (TEAMID)` (default: `-`, ad hoc). */
  signingIdentity?: string;
  /** A property list of entitlements to sign the app with. */
  entitlements?: string;
  /**
   * A provisioning profile embedded before signing, for restricted entitlements.
   * Must match the app's identifier, entitlements and signing identity.
   * Cannot be used with ad-hoc signing. Paths are relative to the project.
   */
  provisioningProfile?: string;
  /**
   * Property lists of entitlements for code among the resources, by its path
   * there, e.g. `{ "bin/server": "server.entitlements.plist" }` for a helper
   * that needs `com.apple.security.cs.allow-jit` under the hardened runtime.
   * Other code keeps the entitlements it is signed with.
   */
  helperEntitlements?: Record<string, string>;
  /** Keys added to the app's Info.plist, replacing MyGo's, e.g. `NSCameraUsageDescription`. */
  infoPlist?: Record<string, unknown>;
  /** The volume name of the disk image (default: the name). */
  dmgTitle?: string;
  /** Notarizes the app and the disk image of production builds. */
  notarize?: NotarizeConfig;
}

/** Credentials of `xcrun notarytool store-credentials <profile>`. */
export interface NotarizeConfig {
  /** The profile holding the credentials. */
  keychainProfile: string;
  /** The keychain holding the profile (default: the login keychain). */
  keychain?: string;
}

/** Windows packaging. */
export interface WindowsConfig {
  /**
   * The name Windows shows the app by: its shortcuts, its entry in
   * Settings > Apps, the installer's title and the executable's product
   * name (default: the name). The executable and the install folder keep
   * the name's, e.g. `天气助手` for an app named `weather`.
   */
  displayName?: string;
  /**
   * The name of the folder the installer installs the app in, in
   * `%LOCALAPPDATA%\Programs` (default: the name), e.g. an ASCII `ZhiLian`
   * for an app named `智链`, whose shortcuts and entry in Settings > Apps
   * keep the name. Changing it moves new installs only.
   */
  installDirName?: string;
  /** A code signing certificate (.pfx); its password comes from `MYGO_WINDOWS_CERTIFICATE_PASSWORD`. */
  certificate?: string;
  /** Instead of `certificate`, a command that signs a file, with `%1` for its path. */
  signCommand?: string;
  /** The time stamping server of `certificate` (default: `http://timestamp.digicert.com`). */
  timestampUrl?: string;
  /**
   * An NSIS script that the installer includes, e.g. `build/installer.nsh`:
   * the macros it defines among `customInit`, `customInstall` and
   * `customUnInstall` run when the installer starts, at the end of the
   * install, and in the uninstall before the app's folder is removed.
   */
  installerInclude?: string;
}

/** Linux packaging. */
/** The iOS app. */
export interface IOSConfig {
  /** The oldest iOS the app runs on (default: `17.0`). */
  minimumSystemVersion?: string;
  /** The ID of the Apple developer team that signs the app for devices. */
  team?: string;
  /** The provisioning profile to sign with: its name, UUID or the path of a `.mobileprovision` (default: the installed one that best fits). */
  provisioningProfile?: string;
  /** What the build is for (default: `app-store`, TestFlight too). */
  distribution?: "app-store" | "ad-hoc" | "development" | "enterprise";
  /** `CFBundleVersion`, which must grow with every upload of a version (default: the version). */
  /** Lets WindowOptions.OwnScene open windows of the system's own on iPad. */
  multipleWindows?: boolean;
  buildNumber?: string;
  /** The orientations the app turns to on iPhones (default: both); iPads take every one. */
  orientations?: ("portrait" | "landscape")[];
  /** Extra keys of Info.plist, which replace MyGo's own, as usage descriptions. */
  infoPlist?: Record<string, unknown>;
  /**
   * The hosts of the app's universal links, which reach `App.OnOpenURL`:
   * `example.com` signs the app with `applinks:example.com`, and a value with
   * its own service, as `webcredentials:example.com`, stays as it is. The
   * provisioning profile needs the capability Associated Domains.
   */
  associatedDomains?: string[];
  /** What the system shows while the app starts (default: the system background). */
  launchScreen?: IOSLaunchScreen;
  /** The app's `UIBackgroundModes`, as `audio`, `fetch` or `remote-notification`, with those its plugins ask for. */
  backgroundModes?: string[];
  /**
   * App extensions built apart, as a Network Extension's packet tunnel built
   * with Xcode, which the app carries in `PlugIns/`, each signed with its own
   * provisioning profile before the app.
   */
  extensions?: IOSExtension[];
}

/** A prebuilt app extension of the iOS app. */
export interface IOSExtension {
  /** The `.appex`, whose identifier starts with the app's and a dot, as `com.example.app.tunnel`. */
  path: string;
  /** The profile to sign it with for `distribution`: its name, UUID or the path of a `.mobileprovision` (default: the installed profile that best fits its identifier and the distribution, with the app's certificate). */
  provisioningProfile?: string;
}

/** The launch screen of the iOS app: at least one of its fields. */
export interface IOSLaunchScreen {
  /** Its color, as `#102030`. */
  background?: string;
  /** A PNG shown centered, at the size of a 3x screen: a logo of 600×600 pixels takes 200×200 points. */
  image?: string;
}

/** The Android app. */
export interface AndroidConfig {
  /** The oldest Android API level the app runs on (default: `26`, Android 8.0). */
  minSdk?: number;
  /** The integer version Google Play orders uploads by (default: from the version, major×10000 + minor×100 + patch). */
  versionCode?: number;
  /** Permissions the manifest declares besides INTERNET, as `android.permission.CAMERA` or `CAMERA`. */
  permissions?: string[];
  /** The keystore of the upload key that signs release builds; its passwords come from `MYGO_ANDROID_KEYSTORE_PASSWORD` and `MYGO_ANDROID_KEY_PASSWORD`. Without one, builds are signed with the debug key. */
  keystore?: string;
  /** The alias of the upload key in the keystore. */
  keyAlias?: string;
  /**
   * The hosts of the app's https deep links, verified by Android, which reach
   * `App.OnOpenURL`: each serves `/.well-known/assetlinks.json` naming the
   * app. `*.example.com` takes its subdomains.
   */
  appLinks?: string[];
  /** The launcher icon of Android 8.0 and later, which the launcher masks and moves. Without it, launchers show `icon` as it is. */
  adaptiveIcon?: AndroidAdaptiveIcon;
  /** The color of the screen the system shows while the app starts, as `#102030` (default: the theme's). */
  splash?: string;
  /**
   * Has the installer extract the app's native libraries to `nativeLibraryDir`,
   * the one place an app may exec its own files from: for an executable that
   * ships as `lib<name>.so` in a native module's `jniLibs` (default: `false`,
   * they stay in the APK).
   */
  extractNativeLibs?: boolean;
}

/** An icon of two layers. */
export interface AndroidAdaptiveIcon {
  /** A square PNG, ideally 432×432 or more, with transparency around what it shows: launchers crop to the middle 72/108 of it. */
  foreground: string;
  /** The color under it, as `#102030`. */
  background: string;
}

export interface LinuxConfig {
  /** `Name <email>`, the maintainer of the Debian and RPM packages and of the PKGBUILD (default: `author` in package.json, else the app's name). */
  maintainer?: string;
  /** A short description, for the desktop entry and the packages. */
  comment?: string;
  /** The categories of the desktop entry (default: `["Utility"]`). */
  categories?: string[];
  /** Packages the app needs besides GTK and WebKitGTK, by package format or as Debian packages mapped to Arch Linux and RPM where MyGo knows them. */
  depends?: string[] | LinuxDepends;
  /**
   * `false` for an app that shows no web page, in a window or a `ui.WebView`,
   * as an app of native UI: its Debian and RPM packages and PKGBUILD depend on GTK alone, and
   * install.sh and `mygo doctor` do not ask for WebKitGTK (default: `true`).
   */
  webview?: boolean;
  /**
   * A command that runs the app, such as `my-app`, or a list of names, such
   * as `["my-app", "ma"]`, which adds a short one: a link in `/usr/bin` from
   * the Debian and RPM packages and the PKGBUILD, and in `~/.local/bin` from install.sh, for each,
   * which says to run the app by the first it links (default: none; the app
   * opens from the applications menu).
   */
  command?: string | string[];
  /**
   * The packages `mygo build` writes (default: all three). The archive with
   * install.sh is always written, as install.sh updates the app from it and
   * the PKGBUILD packages it; `[]` writes it alone.
   */
  formats?: ("deb" | "rpm" | "pkgbuild")[];
}

/**
 * Extra package dependencies for Linux packages, by package format.
 */
export interface LinuxDepends {
  /** Debian packages the app needs besides GTK and WebKitGTK. */
  deb?: string[];
  /** RPM package dependencies or library requirements besides GTK and WebKitGTK. */
  rpm?: string[];
  /** Arch Linux packages the app needs besides GTK and WebKitGTK. */
  arch?: string[];
}
