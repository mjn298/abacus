import { Router } from 'express';
import { UserService } from '../services/user.service';

const router = Router();
const userService = new UserService();

router.post('/users', async (req, res) => {
  const user = await userService.createUser(req.body);
  res.status(201).json(user);
});

export default router;
