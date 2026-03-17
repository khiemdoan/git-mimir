import { greet } from "./greeter";
import { add, multiply } from "./math";

export function main(): void {
  const message = greet("World");
  console.log(message);

  const sum = add(1, 2);
  const product = multiply(3, 4);
  console.log(`${sum} ${product}`);
}

main();
