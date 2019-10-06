import { LoadingSpinner } from '@sourcegraph/react-loading-spinner'
import React from 'react'
import { toDiagnostic } from '../../../../../../shared/src/api/types/diagnostic'
import { ExtensionsControllerProps } from '../../../../../../shared/src/extensions/controller'
import * as GQL from '../../../../../../shared/src/graphql/schema'
import { isErrorLike } from '../../../../../../shared/src/util/errors'
import { DiagnosticListByResource } from '../../../../diagnostics/list/byResource/DiagnosticListByResource'
import { useCampaignDiagnostics } from './useCampaignDiagnostics'
import { ThemeProps } from '../../../../theme'

interface Props extends ExtensionsControllerProps, ThemeProps {
    campaign: Pick<GQL.IExpCampaign, 'id' | 'url'>

    className?: string
}

const LOADING = 'loading' as const

/**
 * The diagnostics in all of a campaign's threads.
 */
export const CampaignDiagnostics: React.FunctionComponent<Props> = ({ campaign, className = '', ...props }) => {
    const [diagnostics] = useCampaignDiagnostics(campaign)
    return (
        <div className={`campaign-diagnostics ${className}`}>
            {diagnostics === LOADING ? (
                <LoadingSpinner className="icon-inline" />
            ) : isErrorLike(diagnostics) ? (
                <div className="alert alert-danger">{diagnostics.message}</div>
            ) : diagnostics.totalCount === 0 ? (
                <span className="text-muted">No diagnostics</span>
            ) : (
                <DiagnosticListByResource
                    {...props}
                    diagnostics={diagnostics.edges.map(e => ({
                        ...e.diagnostic.data,
                        ...toDiagnostic(e.diagnostic.data),
                    }))}
                    listClassName="list-group"
                />
            )}
        </div>
    )
}
