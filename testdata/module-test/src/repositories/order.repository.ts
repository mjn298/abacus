import { PrismaClient } from '@prisma/client';

const prisma = new PrismaClient();

export class OrderRepository {
  async create(data: any) {
    return prisma.order.create({ data });
  }
}
