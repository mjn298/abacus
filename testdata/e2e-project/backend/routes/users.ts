import { Router } from 'express';
import { PrismaClient } from '@prisma/client';
import { listUsersContract } from '../../contracts/users';

const router = Router();
const prisma = new PrismaClient();

router.get('/users', async (req, res) => {
  const users = await prisma.user.findMany();
  res.json(users);
});

router.post('/users', async (req, res) => {
  const user = await prisma.user.create({ data: req.body });
  res.status(201).json(user);
});

router.get('/users/:id', async (req, res) => {
  const user = await prisma.user.findUnique({ where: { id: req.params.id } });
  res.json(user);
});

export default router;
