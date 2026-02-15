using System.Text.Json;
using NUnit.Framework;
using WizardBeardStudio.Rgs.Services;

namespace WizardBeardStudio.Rgs.Tests.Editor
{
    public sealed class RestJsonTests
    {
        [Test]
        public void ParseMeta_WhenNumericResultCodeOk_SetsSuccessTrue()
        {
            using var doc = JsonDocument.Parse("{" +
                                              "\"meta\":{" +
                                              "\"requestId\":\"req-1\"," +
                                              "\"resultCode\":1," +
                                              "\"denialReason\":\"\"," +
                                              "\"serverTime\":\"2026-02-15T00:00:00Z\"}}") ;

            var meta = RestJson.ParseMeta(doc.RootElement);

            Assert.That(meta.Success, Is.True);
            Assert.That(meta.ResultCode, Is.EqualTo("1"));
            Assert.That(meta.RequestId, Is.EqualTo("req-1"));
        }

        [Test]
        public void ParseMeta_WhenStringResultCodeDenied_SetsSuccessFalse()
        {
            using var doc = JsonDocument.Parse("{" +
                                              "\"meta\":{" +
                                              "\"requestId\":\"req-2\"," +
                                              "\"resultCode\":\"RESULT_CODE_DENIED\"," +
                                              "\"denialReason\":\"denied\"," +
                                              "\"serverTime\":\"2026-02-15T00:00:00Z\"}}") ;

            var meta = RestJson.ParseMeta(doc.RootElement);

            Assert.That(meta.Success, Is.False);
            Assert.That(meta.ResultCode, Is.EqualTo("RESULT_CODE_DENIED"));
            Assert.That(meta.DenialReason, Is.EqualTo("denied"));
        }
    }
}
