import { Router } from 'express';

const router = Router();

// Simple GET
router.get('/users', getUsers);

// POST with middleware
router.post('/users', validateBody, createUser);

// Parameterized route
router.get('/users/:id', getUserById);

// PUT with multiple middleware
router.put('/users/:id', requireAuth, validateBody, updateUser);

// DELETE
router.delete('/users/:id', requireAuth, deleteUser);

export default router;
