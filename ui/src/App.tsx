import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import Layout from './components/Layout';
import Applications from './pages/Applications';
import ApplicationCreate from './pages/ApplicationCreate';
import ApplicationEdit from './pages/ApplicationEdit';
import ApplicationDetail from './pages/ApplicationDetail';
import Policies from './pages/Policies';
import PolicyDetail from './pages/PolicyDetail';
import Deployments from './pages/Deployments';
import DeploymentDetail from './pages/DeploymentDetail';
import Environments from './pages/Environments';
import Providers from './pages/Providers';

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<Navigate to="/applications" replace />} />
          <Route path="/applications" element={<Applications />} />
          <Route path="/applications/create" element={<ApplicationCreate />} />
          <Route path="/applications/:name" element={<ApplicationDetail />} />
          <Route path="/applications/:name/edit" element={<ApplicationEdit />} />
          <Route path="/policies" element={<Policies />} />
          <Route path="/policies/:name" element={<PolicyDetail />} />
          <Route path="/deployments" element={<Deployments />} />
          <Route path="/deployments/:id" element={<DeploymentDetail />} />
          <Route path="/environments" element={<Environments />} />
          <Route path="/providers" element={<Providers />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
