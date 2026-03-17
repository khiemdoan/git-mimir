export class Logger {
  info(msg: string): void {
    console.log(`[INFO] ${msg}`);
  }

  error(msg: string): void {
    console.error(`[ERROR] ${msg}`);
  }

  warn(msg: string): void {
    console.warn(`[WARN] ${msg}`);
  }
}
