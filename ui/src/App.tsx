import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import Layout from './components/Layout';
import Applications from './pages/Applications';
import ApplicationDetail from './pages/ApplicationDetail';
import Policies from './pages/Policies';
import PolicyDetail from './pages/PolicyDetail';
import Deployments from './pages/Deployments';
import DeploymentDetail from './pages/DeploymentDetail';
import Providers from './pages/Providers';

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<Navigate to="/applications" replace />} />
          <Route path="/applications" element={<Applications />} />
          <Route path="/applications/:name" element={<ApplicationDetail />} />
          <Route path="/policies" element={<Policies />} />
          <Route path="/policies/:name" element={<PolicyDetail />} />
          <Route path="/deployments" element={<Deployments />} />
          <Route path="/deployments/:id" element={<DeploymentDetail />} />
          <Route path="/providers" element={<Providers />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
