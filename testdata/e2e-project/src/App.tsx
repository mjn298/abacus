import React from 'react';
import { Route } from 'react-router-dom';

const Home = () => <div>Home</div>;
const UserList = () => <div>Users</div>;
const UserDetail = () => <div>User Detail</div>;

export function AppRoutes() {
  return (
    <>
      <Route path="/" element={<Home />} />
      <Route path="/users" element={<UserList />} />
      <Route path="/users/:id" element={<UserDetail />} />
    </>
  );
}
