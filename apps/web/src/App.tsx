import React, { useState } from 'react';
import { AppProvider, useApp } from './context/AppContext';
import { AppShell } from './shell/AppShell';
import { CustomerShell } from './shell/CustomerShell';
import { StaffLoginView } from './features/auth/StaffLoginView';
import { DashboardView } from './features/DashboardView';
import { TransactionsView } from './features/TransactionsView';
import { ReconciliationView } from './features/ReconciliationView';
import { ExceptionsView } from './features/ExceptionsView';
import { InvestigationsView } from './features/InvestigationsView';
import { ReversalsView } from './features/ReversalsView';
import { AuditView } from './features/AuditView';
import { StorybookView } from './features/StorybookView';
import { AgentIntakeView } from './features/AgentIntakeView';
import { ReverseItView } from './features/reverseit/ReverseItView';
import { RulesAndHealthView } from './features/RulesAndHealthView';
import { ExecutiveReportsView } from './features/ExecutiveReportsView';
import { ModelDriftAndShadowView } from './features/ModelDriftAndShadowView';

const MainViewRouter: React.FC = () => {
  const { activeView } = useApp();

  switch (activeView) {
    case 'dashboard':
      return <DashboardView />;
    case 'agent-intake':
      return <AgentIntakeView />;
    case 'reverseit':
      return <ReverseItView />;
    case 'transactions':
      return <TransactionsView />;
    case 'reconciliation':
      return <ReconciliationView />;
    case 'exceptions':
      return <ExceptionsView />;
    case 'investigations':
      return <InvestigationsView />;
    case 'reversals':
      return <ReversalsView />;
    case 'audit':
      return <AuditView />;
    case 'rules-health':
      return <RulesAndHealthView />;
    case 'executive-reports':
      return <ExecutiveReportsView />;
    case 'model-drift':
      return <ModelDriftAndShadowView />;
    case 'storybook':
      return <StorybookView />;
    default:
      return <DashboardView />;
  }
};

const CustomerPortal: React.FC = () => {
  return (
    <CustomerShell>
      <ReverseItView />
    </CustomerShell>
  );
};

const AppContent: React.FC = () => {
  const { portalMode, isAuthenticated } = useApp();

  // 1. Customer Self-Service Portal (Completely segregated from bank operations)
  if (portalMode === 'customer') {
    return <CustomerPortal />;
  }

  // 2. Bank Staff Operations Portal (Requires staff authentication)
  if (!isAuthenticated) {
    return <StaffLoginView />;
  }

  // 3. Authenticated Bank Operations Console
  return (
    <AppShell>
      <MainViewRouter />
    </AppShell>
  );
};

export function App() {
  return (
    <AppProvider>
      <AppContent />
    </AppProvider>
  );
}

export default App;
