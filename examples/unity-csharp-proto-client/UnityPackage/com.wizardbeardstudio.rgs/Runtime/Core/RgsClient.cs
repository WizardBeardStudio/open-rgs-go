using System.Threading;
using System.Threading.Tasks;
using WizardBeardStudio.Rgs.Auth;
using WizardBeardStudio.Rgs.Metadata;
using WizardBeardStudio.Rgs.Models;
using WizardBeardStudio.Rgs.Services;

namespace WizardBeardStudio.Rgs.Core
{
    public sealed class RgsClient
    {
        private readonly RgsClientConfig _config;
        private readonly RgsAuthService _auth;
        private readonly SessionsClient _sessions;
        private readonly LedgerClient _ledger;
        private readonly WageringClient _wagering;
        private readonly EventsClient _events;
        private readonly ReportingClient _reporting;

        public RgsClient(
            RgsClientConfig config,
            RgsAuthService auth,
            SessionsClient sessions,
            LedgerClient ledger,
            WageringClient wagering,
            EventsClient events,
            ReportingClient reporting)
        {
            _config = config;
            _auth = auth;
            _sessions = sessions;
            _ledger = ledger;
            _wagering = wagering;
            _events = events;
            _reporting = reporting;
        }

        public Task<LoginResult> LoginAsync(string playerId, string pin, CancellationToken cancellationToken)
            => _auth.LoginPlayerAsync(playerId, pin, cancellationToken);

        public Task<LoginResult> RefreshTokenAsync(CancellationToken cancellationToken)
            => _auth.RefreshTokenAsync(cancellationToken);

        public Task<OperationResult> LogoutAsync(CancellationToken cancellationToken)
            => _auth.LogoutAsync(cancellationToken);

        public Task<BalanceResult> GetBalanceAsync(string accountId, CancellationToken cancellationToken)
            => _ledger.GetBalanceAsync(accountId, cancellationToken);

        public Task<SessionResult> StartSessionAsync(string deviceId, CancellationToken cancellationToken)
            => _sessions.StartSessionAsync(deviceId, cancellationToken);

        public Task<WagerResult> PlaceWagerAsync(string gameId, long amountMinor, string currency, string? idempotencyKey, CancellationToken cancellationToken)
        {
            var idem = idempotencyKey;
            if (string.IsNullOrWhiteSpace(idem) && _config.autoGenerateIdempotencyKey)
            {
                idem = IdempotencyKeyFactory.Create();
            }
            if (string.IsNullOrWhiteSpace(idem))
            {
                throw new RgsValidationException("Idempotency key is required for financial operations.");
            }
            return _wagering.PlaceWagerAsync(gameId, amountMinor, currency, idem, cancellationToken);
        }

        public Task<OutcomeResult> SettleWagerAsync(string wagerId, long payoutMinor, string currency, CancellationToken cancellationToken)
            => _wagering.SettleWagerAsync(wagerId, payoutMinor, currency, cancellationToken);

        public Task<OperationResult> SubmitSignificantEventAsync(string eventCode, string description, CancellationToken cancellationToken)
            => _events.SubmitSignificantEventAsync(eventCode, description, cancellationToken);

        public Task<OperationResult> GenerateReportAsync(string reportType, string interval, CancellationToken cancellationToken)
            => _reporting.GenerateReportAsync(reportType, interval, cancellationToken);

        public Task<OperationResult> EndSessionAsync(string sessionId, CancellationToken cancellationToken)
            => _sessions.EndSessionAsync(sessionId, cancellationToken);
    }
}
