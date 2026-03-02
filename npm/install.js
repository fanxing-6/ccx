const fs = require("fs");
const path = require("path");
const https = require("https");
const http = require("http");
const { execSync } = require("child_process");

const pkg = require("./package.json");
const version = pkg.version;

const GITHUB_API_URL = "https://api.github.com/repos/fanxing-6/ccx/releases/latest";

const BINARY_NAME = "ccx";
const DOWNLOAD_URL = `https://github.com/fanxing-6/ccx/releases/download/v${version}/ccx_linux_amd64.tar.gz`;

const binDir = path.join(__dirname, "bin");
const binPath = path.join(binDir, BINARY_NAME);
const tmpFile = path.join(__dirname, "ccx.tar.gz");

function followRedirects(url, maxRedirects = 5) {
  return new Promise((resolve, reject) => {
    if (maxRedirects <= 0) {
      reject(new Error("重定向次数过多"));
      return;
    }

    const client = url.startsWith("https") ? https : http;
    client
      .get(url, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          followRedirects(res.headers.location, maxRedirects - 1).then(resolve, reject);
          return;
        }
        if (res.statusCode !== 200) {
          reject(new Error(`下载失败: HTTP ${res.statusCode} — ${url}`));
          return;
        }
        resolve(res);
      })
      .on("error", reject);
  });
}

const BINARY_NAME = "ccx";
const DOWNLOAD_URL = `https://github.com/fanxing-6/ccx/releases/download/v${version}/ccx_linux_amd64.tar.gz`;

const binDir = path.join(__dirname, "bin");
const binPath = path.join(binDir, BINARY_NAME);
const tmpFile = path.join(__dirname, "ccx.tar.gz");

async function install() {
  if (process.platform !== "linux" || process.arch !== "x64") {
    console.error(`ccx 目前仅支持 linux/x64，当前平台: ${process.platform}/${process.arch}`);
    process.exit(1);
  }

  console.log(`正在下载 ccx v${version}...`);

  fs.mkdirSync(binDir, { recursive: true });

  // 下载 tar.gz 到临时文件
  const res = await followRedirects(DOWNLOAD_URL);
  await new Promise((resolve, reject) => {
    const ws = fs.createWriteStream(tmpFile);
    res.pipe(ws);
    ws.on("finish", resolve);
    ws.on("error", reject);
    res.on("error", reject);
  });

  // 用系统 tar 解压（Linux 环境必定有 tar）
  execSync(`tar -xzf "${tmpFile}" -C "${binDir}" ${BINARY_NAME}`, { stdio: "inherit" });

  // 清理临时文件
  fs.unlinkSync(tmpFile);

  fs.chmodSync(binPath, 0o755);
  console.log(`ccx v${version} 安装成功: ${binPath}`);
}

async function checkLatestVersion() {
  return new Promise((resolve) => {
    https
      .get(
        GITHUB_API_URL,
        {
          headers: {
            "User-Agent": "claude-ccx-installer",
          },
        },
        (res) => {
          if (res.statusCode !== 200) {
            resolve(null);
            return;
          }
          let data = "";
          res.on("data", (chunk) => (data += chunk));
          res.on("end", () => {
            try {
              const release = JSON.parse(data);
              const latestVersion = release.tag_name.replace(/^v/, "");
              const currentVersion = version;
              if (latestVersion !== currentVersion) {
                resolve(latestVersion);
              } else {
                resolve(null);
              }
            } catch {
              resolve(null);
            }
          });
        }
      )
      .on("error", () => resolve(null))
      .setTimeout(3000, function () {
        this.destroy();
        resolve(null);
      });
  });
}

async function main() {
  await install();

  // 异步检查更新，不阻塞安装
  const latestVersion = await checkLatestVersion();
  if (latestVersion) {
    console.log("\n" + "=".repeat(50));
    console.log(`⚠️  发现新版本: v${latestVersion}`);
    console.log(`   当前版本: v${version}`);
    console.log(`\n   更新命令: npm install -g claude-ccx`);
    console.log("=".repeat(50));
  }
}

main().catch((err) => {
  console.error(`安装 ccx 失败: ${err.message}`);
  process.exit(1);
});
