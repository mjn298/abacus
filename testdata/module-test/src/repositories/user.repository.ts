import { PrismaClient } from '@prisma/client';

const prisma = new PrismaClient();

export class UserRepository {
  async create(data: any) {
    return prisma.user.create({ data });
  }
}
