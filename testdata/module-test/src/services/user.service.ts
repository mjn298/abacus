import { UserRepository } from '../repositories/user.repository';

export class UserService {
  private repo = new UserRepository();

  async createUser(data: any) {
    return this.repo.create(data);
  }
}
