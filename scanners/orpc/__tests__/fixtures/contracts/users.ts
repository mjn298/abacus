import { oc } from '@orpc/contract';
import { z } from 'zod';

const UserSchema = z.object({
  id: z.string(),
  email: z.string(),
  name: z.string(),
});

const ListUsersInputSchema = z.object({
  page: z.number().optional(),
  limit: z.number().optional(),
});

const ListUsersOutputSchema = z.object({
  users: z.array(UserSchema),
  total: z.number(),
});

const UpdateUserInputSchema = z.object({
  name: z.string().optional(),
  email: z.string().email().optional(),
});

export const listUsers = oc
  .route({
    method: 'GET',
    path: '/users',
    summary: 'List all users',
    tags: ['Users'],
  })
  .input(ListUsersInputSchema)
  .output(ListUsersOutputSchema);

export const getUser = oc
  .route({
    method: 'GET',
    path: '/users/{id}',
    summary: 'Get user by ID',
    tags: ['Users'],
  })
  .output(UserSchema);

export const updateUser = oc
  .route({
    method: 'PUT',
    path: '/users/{id}',
    summary: 'Update a user',
    tags: ['Users'],
  })
  .input(UpdateUserInputSchema)
  .output(UserSchema);

export const deleteUser = oc
  .route({
    method: 'DELETE',
    path: '/users/{id}',
    summary: 'Delete a user',
    tags: ['Users'],
    successStatus: 204,
  });

export const patchUser = oc
  .route({
    method: 'PATCH',
    path: '/users/{id}/status',
    tags: ['Users'],
  })
  .input(UpdateUserInputSchema);
