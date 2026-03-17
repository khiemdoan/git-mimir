import { UserService } from './user-service';
import { Logger } from './logger';

export class UserController {
  private service: UserService;
  private logger: Logger;

  constructor() {
    this.service = new UserService();
    this.logger = new Logger();
  }

  handleGetUser(id: string): object {
    this.logger.info(`Getting user ${id}`);
    return this.service.getUser(id);
  }

  handleCreateUser(data: object): object {
    this.logger.info('Creating user');
    return this.service.createUser(data);
  }
}
