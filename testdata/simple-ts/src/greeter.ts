export function greet(name: string): string {
  return `Hello, ${name}!`;
}

export class Greeter {
  private prefix: string;

  constructor(prefix: string) {
    this.prefix = prefix;
  }

  greet(name: string): string {
    return `${this.prefix}, ${name}!`;
  }
}
