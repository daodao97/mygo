# GitHub Actions

推送标签时，GitHub Actions 工作流可以构建每个平台的应用，为它们签名，再连同各自的[更新](updates.md)上传到一个草稿发布版本，由你来发布。每个平台在各自的运行器上同时构建。MyGo 不需要 cgo，所以 Ubuntu 运行器既能构建 Linux 应用，也能构建 Windows 应用；macOS 应用需要 macOS 运行器，因为代码签名、公证和磁盘映像需要 macOS。

## 工作流

配置里设置了 `updates.github` 时，把下面内容保存为 `.github/workflows/release.yml`：

```yaml
name: Release

on:
  push:
    tags: ["v*"]

permissions:
  contents: write # the draft release, and the uploads to it

jobs:
  draft-release:
    runs-on: ubuntu-latest
    steps:
      - name: Create the draft release
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          if ! gh release view "$GITHUB_REF_NAME" --repo "$GITHUB_REPOSITORY" > /dev/null 2>&1; then
            gh release create "$GITHUB_REF_NAME" --repo "$GITHUB_REPOSITORY" \
              --draft --verify-tag --title "$GITHUB_REF_NAME" --generate-notes
          fi

  build:
    needs: draft-release
    name: ${{ matrix.name }}
    runs-on: ${{ matrix.os }}
    strategy:
      fail-fast: false
      matrix:
        include:
          - { name: macOS, os: macos-latest, platform: "darwin/arm64,darwin/amd64" }
          - { name: Windows, os: ubuntu-latest, platform: "windows/amd64,windows/arm64" }
          - { name: Linux, os: ubuntu-latest, platform: "linux/amd64,linux/arm64" }
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version-file: go.mod
      - uses: oven-sh/setup-bun@v2
      - run: bun install --frozen-lockfile
      - name: Install NSIS, for the Windows installers
        if: matrix.name == 'Windows' && runner.os == 'Linux'
        run: sudo apt-get update && sudo apt-get install -y nsis
      - name: Build and upload
        env:
          GH_TOKEN: ${{ github.token }}
          MYGO_UPDATER_PRIVATE_KEY: ${{ secrets.MYGO_UPDATER_PRIVATE_KEY }}
        run: bun run build -- -platform "${{ matrix.platform }}" -upload
```

[原生 UI](ui/README.md) 项目没有前端，也就没有 Bun：省略 `setup-bun` 和 `bun install` 步骤，改用 `go.mod` 以工具方式固定的 CLI 来构建：

```yaml
        run: go tool mygo build -platform "${{ matrix.platform }}" -upload
```

把给更新签名的密钥，即 `mygo-update.key` 的内容（见[自动更新](updates.md#设置)），添加到仓库的机密：

```sh
gh secret set MYGO_UPDATER_PRIVATE_KEY < path/to/mygo-update.key
```

然后把配置里的 `version` 改成新版本，提交，并推送标签：

```sh
git tag v1.2.0
git push origin v1.2.0
```

创建草稿发布版本的作业会先建好草稿 `v1.2.0`。接着构建作业按矩阵的每一行同时运行一次，各自上传到草稿：macOS 磁盘映像、Windows 安装程序，以及 Linux 归档、Debian 包和安装脚本，都连同各自的更新。检查草稿后发布它：应用随后就会更新到它。

- `mygo build` 会上传到配置里版本对应的发布版本，并使用 `updates` 的 `tagPrefix`（默认是 `v`），无论工作流由哪个标签启动。可以用一个步骤检查两者是否一致，见[下文](#检查版本)。
- 草稿在构建开始前就已存在，所以各次构建都上传到它：没有草稿时，每个 `mygo build -upload` 都会各自创建一个草稿。再次运行工作流会保留草稿，并替换其中的文件。
- `fail-fast: false` 让一个平台失败时其他平台仍会完成。“重新运行失败的作业”只会重新构建那一个，它仍上传到同一个草稿。
- 一行的 `platform` 可以列出多个目标，`mygo build` 会依次构建。每个目标单独一行（例如 `windows/amd64` 和 `windows/arm64`）时，会用到更多运行器，同时构建它们。
- 发布页面会显示 GitHub 根据上次发布版本以来的拉取请求生成的发布说明，你可以在草稿里编辑。更新窗口会显示 `CHANGELOG.md` 中该版本的部分，这部分由更新清单携带。
- 工作流的 `GITHUB_TOKEN` 让运行器上自带的 `gh` 创建发布版本并上传到它。有 `tagPrefix` 时，它还能让 `mygo build` 找到[增量更新](updates.md#增量更新)所基于的更早发布版本。

上述工作流构建的应用可以运行，但签名尚不满足分发要求：macOS 应用使用临时签名（ad hoc），在其他 Mac 上会被 Gatekeeper 阻止；Windows 应用没有签名，SmartScreen 会向用户发出警告。接下来介绍如何签名。

## 给 macOS 应用签名和公证

macOS 这一行需要你的 Developer ID 证书和 Apple 公证服务的凭据（见[签名和公证](distribution.md#签名与公证)）。在“钥匙串访问”中把“Developer ID Application”证书连同私钥导出为带密码的 `.p12` 文件，再添加这些机密：

```sh
base64 -i certificate.p12 | gh secret set MACOS_CERTIFICATE
gh secret set MACOS_CERTIFICATE_PASSWORD   # the password of the .p12
gh secret set APPLE_ID                     # the Apple ID of your developer account
gh secret set APPLE_TEAM_ID                # your team ID, as in the identity's (TEAMID)
gh secret set APPLE_APP_PASSWORD           # an app-specific password of the Apple ID
```

在 [account.apple.com](https://account.apple.com) 的“登录与安全”下创建 App 专用密码。然后在 `Build and upload` 之前，把它们放进 macOS 运行器的钥匙串：

```yaml
      - name: Import the certificate and the notary credentials
        if: runner.os == 'macOS'
        env:
          MACOS_CERTIFICATE: ${{ secrets.MACOS_CERTIFICATE }}
          MACOS_CERTIFICATE_PASSWORD: ${{ secrets.MACOS_CERTIFICATE_PASSWORD }}
          APPLE_ID: ${{ secrets.APPLE_ID }}
          APPLE_TEAM_ID: ${{ secrets.APPLE_TEAM_ID }}
          APPLE_APP_PASSWORD: ${{ secrets.APPLE_APP_PASSWORD }}
        run: |
          keychain="$RUNNER_TEMP/signing.keychain-db"
          password="$(openssl rand -base64 24)"
          security create-keychain -p "$password" "$keychain"
          security set-keychain-settings -lut 21600 "$keychain"
          security unlock-keychain -p "$password" "$keychain"
          echo "$MACOS_CERTIFICATE" | base64 --decode > "$RUNNER_TEMP/certificate.p12"
          security import "$RUNNER_TEMP/certificate.p12" -k "$keychain" \
            -P "$MACOS_CERTIFICATE_PASSWORD" -T /usr/bin/codesign
          rm "$RUNNER_TEMP/certificate.p12"
          security set-key-partition-list -S apple-tool:,apple: -s -k "$password" "$keychain"
          security list-keychains -d user -s "$keychain" login.keychain
          xcrun notarytool store-credentials notary --keychain "$keychain" \
            --apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" --password "$APPLE_APP_PASSWORD"
          echo "NOTARY_KEYCHAIN=$keychain" >> "$GITHUB_ENV"
```

钥匙串只在作业运行期间存在，`codesign` 能从中找到证书。配置负责用证书签名，公证则使用钥匙串中保存的配置：在工作流里用那个钥匙串，在你自己的 Mac 上则用登录钥匙串（那里没有设置 `NOTARY_KEYCHAIN`）：

```ts
export default defineConfig({
  macos: {
    signingIdentity: "Developer ID Application: Jane Doe (TEAMID)",
    notarize: { keychainProfile: "notary", keychain: process.env.NOTARY_KEYCHAIN },
  },
});
```

随后 `mygo build` 会给应用签名，先后把应用和磁盘映像提交给公证服务并等待结果（每次通常几分钟），再附加公证票据。也可以用 App Store Connect API 密钥代替 Apple ID：用 `--key`、`--key-id` 和 `--issuer` 把密钥文件、ID 和颁发者提供给 `notarytool store-credentials`。

## 给 Windows 应用签名

在 Ubuntu 运行器上，`osslsigncode` 会用 `.pfx` 证书给 Windows 应用及其安装程序签名。把证书和密码添加到机密：

```sh
base64 -i code-signing.pfx | gh secret set WINDOWS_CERTIFICATE
gh secret set WINDOWS_CERTIFICATE_PASSWORD
```

在 Windows 这一行，随 NSIS 一起安装 `osslsigncode`，把证书写入文件，再把它的路径和密码提供给构建：

```yaml
      - name: Install NSIS and osslsigncode
        if: matrix.name == 'Windows' && runner.os == 'Linux'
        run: sudo apt-get update && sudo apt-get install -y nsis osslsigncode
      - name: Write the certificate
        if: matrix.name == 'Windows'
        env:
          WINDOWS_CERTIFICATE: ${{ secrets.WINDOWS_CERTIFICATE }}
        run: |
          echo "$WINDOWS_CERTIFICATE" | base64 --decode > "$RUNNER_TEMP/code-signing.pfx"
          echo "WINDOWS_CERTIFICATE_FILE=$RUNNER_TEMP/code-signing.pfx" >> "$GITHUB_ENV"
      - name: Build and upload
        env:
          GH_TOKEN: ${{ github.token }}
          MYGO_UPDATER_PRIVATE_KEY: ${{ secrets.MYGO_UPDATER_PRIVATE_KEY }}
          MYGO_WINDOWS_CERTIFICATE_PASSWORD: ${{ secrets.WINDOWS_CERTIFICATE_PASSWORD }}
        run: bun run build -- -platform "${{ matrix.platform }}" -upload
```

```ts
export default defineConfig({
  windows: { certificate: process.env.WINDOWS_CERTIFICATE_FILE },
});
```

第一个步骤会替换安装 NSIS 的步骤。在你自己的机器上构建时，如果没有设置这个变量，就不会签名。

### 在 Windows 上签名

私钥保存在硬件令牌或云服务（例如 Azure Trusted Signing）中的证书，要用 `signCommand` 来签名（见[代码签名](distribution.md#代码签名)），而它通常需要 `signtool`。那就改在 Windows 运行器上构建 Windows 应用：这类运行器带有 `signtool`，`mygo build` 也会自己下载 NSIS；安装 NSIS 的步骤只用于 Linux 运行器：

```yaml
          - { name: Windows, os: windows-latest, platform: "windows/amd64,windows/arm64" }
```

在 Windows 这一行的 `Build and upload` 之前加一个步骤，按签名服务的文档登录。Windows 运行器上的步骤默认在 PowerShell 中运行，除非设置了 `shell: bash`。

每次在新的运行器上运行时，`mygo build` 都会下载 NSIS。可以在 `Build and upload` 之前加一个步骤，改为把它保存在 Actions 缓存中，这样构建既不用下载，也不用访问提供下载的主机。锁文件固定了 mygo-cli 的版本，mygo-cli 又固定了 NSIS 的版本：

```yaml
      - name: Cache NSIS
        if: runner.os == 'Windows'
        uses: actions/cache@v6
        with:
          path: ~\AppData\Local\mygo\nsis-*
          key: nsis-${{ hashFiles('bun.lock') }}
```

在原生 UI 项目中，`go.mod` 固定了 CLI：缓存键改用 `hashFiles('go.sum')`。

## 检查版本

标签和配置里的版本不一致时，构建就会以另一个版本号发布。把版本放在 package.json 中，让配置读取它（见[计算出的配置](configuration.md#计算出的配置)）：

```ts
import pkg from "./package.json" with { type: "json" };

export default defineConfig({
  version: pkg.version,
});
```

并让创建草稿发布版本的作业在创建草稿之前检查版本，这样标签不对就什么都不会构建：

```yaml
      - uses: actions/checkout@v7
      - name: Check the version
        run: test "$GITHUB_REF_NAME" = "v$(jq -r .version package.json)"
```

原生 UI 项目的版本在 `mygo.json` 中：改用 `jq -r .version mygo.json` 检查。

## 发布版本

看过草稿后自己发布，或者让工作流在每个作业都上传完后自动发布：

```yaml
  publish:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - env:
          GH_TOKEN: ${{ github.token }}
        run: gh release edit "$GITHUB_REF_NAME" --repo "$GITHUB_REPOSITORY" --draft=false
```

使用 `desktop-v` 这样的 `tagPrefix` 时，用带该前缀的标签触发工作流（`tags: ["desktop-v*"]`），检查标签是否与 `desktop-v` 加版本号一致，并用 `--latest=false` 创建并发布版本，避免替换仓库中其他系列的最新发布版本（见[构建和发布](updates.md#构建与发布)）。

发布预发布版本的标签（如 `v1.3.0-beta.1`）时，也要加上 `--prerelease`，这样只有用户选择了预发布的应用才会更新到它（[预发布](updates.md#预发布)）。

## 发布到 S3

使用 `updates.s3` 时，`mygo build -upload` 会改为上传到存储桶（见[发布到 S3](updates.md#发布到-s3)）。用存储桶的凭据代替 `GH_TOKEN` 提供给作业：

```yaml
        env:
          AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
          AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          MYGO_UPDATER_PRIVATE_KEY: ${{ secrets.MYGO_UPDATER_PRIVATE_KEY }}
```

这时没有草稿发布版本：省略创建草稿的作业，以及构建作业的 `needs`。每个平台的作业一上传清单，对应的更新就发布了，工作流也不需要 `contents: write` 权限。

## 没有发布版本时

没有 `updates` 配置时，`-upload` 无处可传。构建时不要加 `-upload`，把安装程序保留为本次运行的产物；每次推送都可以这样做，用来检查应用能否构建：

```yaml
      - run: bun run build -- -platform "${{ matrix.platform }}"
      - uses: actions/upload-artifact@v7
        with:
          name: ${{ matrix.name }}
          path: |
            build/*/*.dmg
            build/*/*-setup-*.exe
            build/*/*.deb
            build/*/*.rpm
            build/*/*.tar.gz
```

`build` 是新项目配置里的 `out`。
