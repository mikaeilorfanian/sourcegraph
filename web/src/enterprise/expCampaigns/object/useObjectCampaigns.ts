import { useCallback, useEffect, useState } from 'react'
import { map, startWith } from 'rxjs/operators'
import { dataOrThrowErrors, gql } from '../../../../../shared/src/graphql/graphql'
import * as GQL from '../../../../../shared/src/graphql/schema'
import { asError, ErrorLike } from '../../../../../shared/src/util/errors'
import { queryGraphQL } from '../../../backend/graphql'

const LOADING: 'loading' = 'loading'

type Result = typeof LOADING | GQL.IExpCampaignConnection | ErrorLike

/**
 * A React hook that observes all campaigns that contain the given object (queried from the GraphQL
 * API).
 *
 * @param object The object whose campaigns to observe.
 */
export const useObjectCampaigns = (object: Pick<GQL.ExpCampaignNode, 'id'>): [Result, () => void] => {
    const [updateSequence, setUpdateSequence] = useState(0)
    const incrementUpdateSequence = useCallback(() => setUpdateSequence(updateSequence + 1), [updateSequence])

    const [result, setResult] = useState<Result>(LOADING)
    useEffect(() => {
        const subscription = queryGraphQL(
            gql`
                query ObjectCampaigns($object: ID!) {
                    expCampaigns(object: $object) {
                        nodes {
                            id
                            name
                            url
                        }
                        totalCount
                    }
                }
            `,
            { object: object.id }
        )
            .pipe(
                map(dataOrThrowErrors),
                map(data => data.expCampaigns),
                startWith(LOADING)
            )
            .subscribe(setResult, err => setResult(asError(err)))
        return () => subscription.unsubscribe()
    }, [object, updateSequence])
    return [result, incrementUpdateSequence]
}
