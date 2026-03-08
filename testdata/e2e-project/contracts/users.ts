import { oc } from '@orpc/contract';
import { z } from 'zod';

const UserSchema = z.object({
  id: z.string(),
  email: z.string(),
  name: z.string().optional(),
});

export const listUsersContract = oc
  .route({
    method: 'GET',
    path: '/users',
    summary: 'List all users',
  })
  .output(z.array(UserSchema));

export const getUserContract = oc
  .route({
    method: 'GET',
    path: '/users/{id}',
    summary: 'Get user by ID',
  })
  .output(UserSchema);

export const createUserContract = oc
  .route({
    method: 'POST',
    path: '/users',
    summary: 'Create a user',
  })
  .input(z.object({ email: z.string(), name: z.string().optional() }))
  .output(UserSchema);
