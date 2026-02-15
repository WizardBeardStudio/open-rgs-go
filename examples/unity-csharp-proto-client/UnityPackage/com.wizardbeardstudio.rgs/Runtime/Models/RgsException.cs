using System;

namespace WizardBeardStudio.Rgs.Models
{
    public class RgsException : Exception
    {
        public RgsException(string message) : base(message)
        {
        }
    }

    public sealed class RgsValidationException : RgsException
    {
        public RgsValidationException(string message) : base(message)
        {
        }
    }

    public sealed class RgsDenialException : RgsException
    {
        public string ResultCode { get; }
        public string DenialReason { get; }

        public RgsDenialException(string resultCode, string denialReason)
            : base($"RGS denied request: {resultCode} {denialReason}")
        {
            ResultCode = resultCode;
            DenialReason = denialReason;
        }
    }
}
