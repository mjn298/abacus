import { Router } from 'express';
import { OrderService } from '../services/order.service';

const router = Router();
const orderService = new OrderService();

router.post('/orders', async (req, res) => {
  const order = await orderService.createOrder(req.body);
  res.status(201).json(order);
});

export default router;
