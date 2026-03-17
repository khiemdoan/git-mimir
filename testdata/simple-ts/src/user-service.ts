import { add } from './math';

export interface User {
  id: string;
  name: string;
  age: number;
}

export class UserService {
  private users: Map<string, User> = new Map();

  getUser(id: string): User | undefined {
    return this.users.get(id);
  }

  createUser(data: any): User {
    const user: User = {
      id: String(add(Date.now(), 1)),
      name: data.name,
      age: data.age,
    };
    this.users.set(user.id, user);
    return user;
  }
}
