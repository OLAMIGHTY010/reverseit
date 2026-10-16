import React, { useState } from 'react';
import { ReverseItHeader } from './ReverseItHeader';
import { S1ReportProblem } from './S1ReportProblem';
import { S2PasteUpload } from './S2PasteUpload';
import { S3CheckDetails } from './S3CheckDetails';
import { S4MatchCandidates } from './S4MatchCandidates';
import { S5WhatWentWrong } from './S5WhatWentWrong';
import { S6YourDeadline } from './S6YourDeadline';
import { S7ReviewSend } from './S7ReviewSend';
import { S8CaseTracker } from './S8CaseTracker';
import { S9EscalationLadder } from './S9EscalationLadder';
import { MyCasesView } from './MyCasesView';
import {
  INITIAL_CUSTOMER_CASES,
  REGULATORY_CLOCKS,
  SAMPLE_NIGERIAN_ALERTS,
} from './reverseitData';
import type {
  ReverseItStep,
  IntakeMode,
  ParsedAlertDetails,
  MatchedTransactionCandidate,
  DisputeType,
  RegulatoryClockInfo,
  CustomerDisputeCase,
} from './reverseitTypes';
import { useApp } from '../../context/AppContext';

export const ReverseItView: React.FC = () => {
  const { addToast } = useApp();

  const [currentStep, setCurrentStep] = useState<ReverseItStep>('S1_REPORT_PROBLEM');
  const [intakeMode, setIntakeMode] = useState<IntakeMode>('PASTE_TEXT');
  const [parsedDetails, setParsedDetails] = useState<ParsedAlertDetails>(
    SAMPLE_NIGERIAN_ALERTS[0].parsed
  );
  const [selectedCandidate, setSelectedCandidate] =
    useState<MatchedTransactionCandidate | null>(null);
  const [disputeType, setDisputeType] = useState<DisputeType>('failed_transfer');
  const [clockInfo, setClockInfo] = useState<RegulatoryClockInfo>(
    REGULATORY_CLOCKS.failed_transfer
  );
  const CUSTOMER_SESSION_CASES_KEY = 'smartreversal_customer_my_cases';

  // Customers only see their own submitted cases. Never other customers' reports!
  const [cases, setCases] = useState<CustomerDisputeCase[]>(() => {
    try {
      const saved = localStorage.getItem(CUSTOMER_SESSION_CASES_KEY);
      return saved ? JSON.parse(saved) : [];
    } catch {
      return [];
    }
  });
  const [selectedCase, setSelectedCase] = useState<CustomerDisputeCase | null>(null);

  // S1 -> S2
  const handleSelectIntakeMode = (mode: IntakeMode) => {
    setIntakeMode(mode);
    setCurrentStep('S2_PASTE_UPLOAD');
  };

  // S2 -> S3
  const handleAlertParsed = (parsed: ParsedAlertDetails) => {
    setParsedDetails(parsed);
    setCurrentStep('S3_CHECK_DETAILS');
  };

  // S3 -> S4
  const handleConfirmDetails = (confirmed: ParsedAlertDetails) => {
    setParsedDetails(confirmed);
    setCurrentStep('S4_MATCH_CANDIDATES');
  };

  // S4 -> S5
  const handleCandidateSelected = (candidate: MatchedTransactionCandidate | null) => {
    setSelectedCandidate(candidate);
    setCurrentStep('S5_WHAT_WENT_WRONG');
  };

  // S5 -> S6
  const handleDisputeSelected = (type: DisputeType) => {
    setDisputeType(type);
    setClockInfo(REGULATORY_CLOCKS[type]);
    setCurrentStep('S6_YOUR_DEADLINE');
  };

  // S6 -> S7
  const handleProceedToComplaint = (clock: RegulatoryClockInfo) => {
    setClockInfo(clock);
    setCurrentStep('S7_REVIEW_SEND');
  };

  // S7 Submission Complete -> Add Case & Navigate
  const handleComplaintSubmitted = (caseNumber: string, complaintText: string) => {
    const newCase: CustomerDisputeCase = {
      id: `case-cus-${Date.now()}`,
      caseNumber,
      trackingId: `TRK-${Math.floor(100000 + Math.random() * 900000)}`,
      customerName: 'Verified Account Holder',
      maskedAccount: `•••• ${parsedDetails.accountTail}`,
      amount: parsedDetails.amount,
      currency: parsedDetails.currency,
      disputeType,
      disputeLabel: disputeType.toUpperCase().replace(/_/g, ' '),
      channel: parsedDetails.channel || 'NIP',
      reference: parsedDetails.reference || selectedCandidate?.reference || 'N/A',
      narration: parsedDetails.narration,
      reportedAt: new Date().toISOString(),
      regulatoryDeadline: new Date(Date.now() + clockInfo.regulatoryHours * 3600000).toISOString(),
      bankTargetDeadline: new Date(Date.now() + clockInfo.bankTargetHours * 3600000).toISOString(),
      clockStatus: 'ON_TRACK',
      timeRemainingText: `${clockInfo.bankTargetHours}h 00m left (Bank Target)`,
      status: 'DETECTED',
      statusLabel: 'Detected (Under Review)',
      customerSafeStatusText:
        'We noticed an unsuccessful transaction on your account. We are checking it and will update you. No action is needed now.',
      complaintLetter: complaintText,
      isOverdue: false,
      history: [
        {
          stage: 'REPORTED',
          label: 'Dispute Filed (CUS001)',
          timestamp: new Date().toLocaleTimeString() + ' WAT',
          details: 'Alert intake verified and queued in the exception workbench.',
          verified: true,
        },
        {
          stage: 'INVESTIGATING',
          label: 'Switch Query Scheduled',
          timestamp: 'Queued',
          details: 'Authoritative transaction status query (TSQ) scheduled.',
          verified: false,
        },
        {
          stage: 'REVERSAL_INITIATED',
          label: 'Automated 8-Check Gate',
          timestamp: 'Pending',
          details: 'Will run upon confirmed switch failure.',
          verified: false,
        },
        {
          stage: 'VERIFIED_RESOLVED',
          label: 'Funds Restoration',
          timestamp: 'Pending',
          details: 'Credit posting verified in core ledger.',
          verified: false,
        },
      ],
    };

    setCases((prev) => {
      const updated = [newCase, ...prev];
      try {
        localStorage.setItem(CUSTOMER_SESSION_CASES_KEY, JSON.stringify(updated));
      } catch {
        // silent
      }
      return updated;
    });
    setSelectedCase(newCase);
    setCurrentStep('S8_CASE_TRACKER');
    addToast({
      type: 'success',
      title: 'Dispute Queued as CUS001',
      message: `Case ${caseNumber} registered with ${clockInfo.bankTargetLabel}.`,
    });
  };

  // Case Selection
  const handleSelectCase = (c: CustomerDisputeCase) => {
    setSelectedCase(c);
    setCurrentStep('S8_CASE_TRACKER');
  };

  return (
    <div className="space-y-6">
      {/* ReverseIt Persistent Navigation Header */}
      <ReverseItHeader
        currentStep={currentStep}
        activeCaseCount={cases.length}
        onNavigateToMyCases={() => setCurrentStep('MY_CASES')}
        onStartNewReport={() => setCurrentStep('S1_REPORT_PROBLEM')}
      />

      {/* Screen Routing */}
      {currentStep === 'S1_REPORT_PROBLEM' && (
        <S1ReportProblem
          onSelectMode={handleSelectIntakeMode}
          onViewMyCases={() => setCurrentStep('MY_CASES')}
          activeCaseCount={cases.length}
        />
      )}

      {currentStep === 'S2_PASTE_UPLOAD' && (
        <S2PasteUpload
          mode={intakeMode}
          onBack={() => setCurrentStep('S1_REPORT_PROBLEM')}
          onAlertParsed={handleAlertParsed}
        />
      )}

      {currentStep === 'S3_CHECK_DETAILS' && (
        <S3CheckDetails
          parsed={parsedDetails}
          onBack={() => setCurrentStep('S2_PASTE_UPLOAD')}
          onConfirmDetails={handleConfirmDetails}
        />
      )}

      {currentStep === 'S4_MATCH_CANDIDATES' && (
        <S4MatchCandidates
          parsed={parsedDetails}
          onBack={() => setCurrentStep('S3_CHECK_DETAILS')}
          onCandidateSelected={handleCandidateSelected}
        />
      )}

      {currentStep === 'S5_WHAT_WENT_WRONG' && (
        <S5WhatWentWrong
          candidate={selectedCandidate}
          onBack={() => setCurrentStep('S4_MATCH_CANDIDATES')}
          onDisputeSelected={handleDisputeSelected}
        />
      )}

      {currentStep === 'S6_YOUR_DEADLINE' && (
        <S6YourDeadline
          disputeType={disputeType}
          onBack={() => setCurrentStep('S5_WHAT_WENT_WRONG')}
          onProceedToComplaint={handleProceedToComplaint}
        />
      )}

      {currentStep === 'S7_REVIEW_SEND' && (
        <S7ReviewSend
          parsed={parsedDetails}
          candidate={selectedCandidate}
          disputeType={disputeType}
          clock={clockInfo}
          onBack={() => setCurrentStep('S6_YOUR_DEADLINE')}
          onComplaintSubmitted={handleComplaintSubmitted}
        />
      )}

      {currentStep === 'S8_CASE_TRACKER' && selectedCase && (
        <S8CaseTracker
          disputeCase={selectedCase}
          onBack={() => setCurrentStep('MY_CASES')}
          onViewEscalation={() => setCurrentStep('S9_ESCALATION_LADDER')}
        />
      )}

      {currentStep === 'S9_ESCALATION_LADDER' && selectedCase && (
        <S9EscalationLadder
          disputeCase={selectedCase}
          onBack={() => setCurrentStep('S8_CASE_TRACKER')}
        />
      )}

      {currentStep === 'MY_CASES' && (
        <MyCasesView
          cases={cases}
          onSelectCase={handleSelectCase}
          onStartNewReport={() => setCurrentStep('S1_REPORT_PROBLEM')}
        />
      )}
    </div>
  );
};
