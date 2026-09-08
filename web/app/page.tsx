import { AdFlowConsole } from '@/components/adflow-console';
import { AuthGate } from '@/components/auth-gate';

export default function Home() {
  return (
    <AuthGate>
      <AdFlowConsole />
    </AuthGate>
  );
}
