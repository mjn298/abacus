import { OrderRepository } from '../repositories/order.repository';

export class OrderService {
  private repo = new OrderRepository();

  async createOrder(data: any) {
    return this.repo.create(data);
  }
}
