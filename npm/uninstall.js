const path = require("path");
const os = require("os");

const configDir = path.join(os.homedir(), ".config", "ccx");

console.log("");
console.log("╔════════════════════════════════════════════╗");
console.log("║  ccx 已卸载                                ║");
console.log("║                                            ║");
console.log("║  本地配置文件未删除（含 Gitee Token）：     ║");
console.log(`║  ${configDir.padEnd(42)}║`);
console.log("║                                            ║");
console.log("║  如需彻底清理，请手动执行：                 ║");
console.log(`║  rm -rf ${configDir.padEnd(34)}║`);
console.log("╚════════════════════════════════════════════╝");
console.log("");
